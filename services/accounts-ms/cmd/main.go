// accounts-ms expone el HTTP API de clientes y cuentas, consume
// transactions.commands desde Kafka para ejecutar débitos/créditos,
// y publica eventos vía outbox.
//
// El main es el "composition root": el único lugar donde se arma todo.
package main

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/j0sehernan/banking-platform-go/pkg/events"
	"github.com/j0sehernan/banking-platform-go/pkg/httpx"
	pkgkafka "github.com/j0sehernan/banking-platform-go/pkg/kafka"
	"github.com/j0sehernan/banking-platform-go/pkg/outbox"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/config"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/domain"
	apphttp "github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/http"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/repo"
	"github.com/j0sehernan/banking-platform-go/services/accounts-ms/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal error", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("starting accounts-ms",
		"http_addr", cfg.HTTPAddr,
		"kafka_brokers", cfg.KafkaBrokers,
	)

	// shutdown coordinado vía context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// === DB ===
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := waitForDB(ctx, pool, logger); err != nil {
		return err
	}

	// === Servicios ===
	accountSvc := service.NewAccountService(pool)
	txHandler := service.NewTransactionHandler(pool, logger)

	// === HTTP ===
	registerDomainErrors()
	handler := apphttp.NewHandler(accountSvc)
	router := apphttp.NewRouter(handler, logger)

	httpSrv := &nethttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// === Kafka ===
	producer := pkgkafka.NewProducer(cfg.KafkaBrokers, logger)
	defer producer.Close()

	dlqProducer := pkgkafka.NewProducer(cfg.KafkaBrokers, logger)
	defer dlqProducer.Close()

	consumer := pkgkafka.NewConsumer(pkgkafka.ConsumerConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.ServiceName,
		Topic:   events.TopicTransactionsCommands,
	}, dlqProducer, logger)
	defer consumer.Close()

	// === Outbox worker ===
	outboxRepo := repo.NewOutboxRepo(pool)
	outboxWorker := outbox.NewWorker(outboxRepo, producer, 500*time.Millisecond, 100, logger)

	// === Goroutines: HTTP, outbox worker, kafka consumer ===
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			logger.Error("http server error", "err", err)
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		outboxWorker.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := consumer.Run(ctx, txHandler.Handle); err != nil {
			logger.Error("consumer stopped with error", "err", err)
		}
	}()

	// === Shutdown ===
	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown error", "err", err)
	}

	wg.Wait()
	logger.Info("accounts-ms stopped")
	return nil
}

// waitForDB hace ping a la DB con retries hasta que responde o el ctx termina.
// Útil cuando el contenedor del ms arranca antes que Postgres esté 100% listo.
func waitForDB(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	const maxAttempts = 30
	for i := 0; i < maxAttempts; i++ {
		if err := pool.Ping(ctx); err == nil {
			return nil
		}
		logger.Info("waiting for database", "attempt", i+1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("database not reachable after 30s")
}

// registerDomainErrors mapea los errores tipados del dominio a HTTP status codes.
// Esto permite que los handlers solo hagan `return err` y el wrapper se
// encargue del status correcto sin código repetido.
func registerDomainErrors() {
	httpx.RegisterDomainError(domain.ErrClientNotFound,
		httpx.NewError(nethttp.StatusNotFound, "client_not_found", "El cliente no existe"))
	httpx.RegisterDomainError(domain.ErrAccountNotFound,
		httpx.NewError(nethttp.StatusNotFound, "account_not_found", "La cuenta no existe"))
	httpx.RegisterDomainError(domain.ErrInsufficientFunds,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "insufficient_funds", "Saldo insuficiente"))
	httpx.RegisterDomainError(domain.ErrInvalidCurrency,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "invalid_currency", "Moneda no soportada"))
	httpx.RegisterDomainError(domain.ErrCurrencyMismatch,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "currency_mismatch", "Las cuentas tienen monedas distintas"))
	httpx.RegisterDomainError(domain.ErrSameAccountTransfer,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "same_account", "No se puede transferir a la misma cuenta"))
	httpx.RegisterDomainError(domain.ErrEmailAlreadyExists,
		httpx.NewError(nethttp.StatusConflict, "email_exists", "El email ya está registrado"))
}
