// transactions-ms expone los endpoints HTTP de transacciones,
// publica TransactionRequested al bus, consume accounts.tx-results
// para cerrar el ciclo del saga, y emite los eventos finales
// (Completed/Rejected) que llm-ms va a procesar.
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
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/config"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/domain"
	apphttp "github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/http"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/repo"
	"github.com/j0sehernan/banking-platform-go/services/transactions-ms/internal/service"
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
	logger.Info("starting transactions-ms",
		"http_addr", cfg.HTTPAddr,
		"kafka_brokers", cfg.KafkaBrokers,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := waitForDB(ctx, pool, logger); err != nil {
		return err
	}

	txSvc := service.NewTransactionService(pool)
	resultHandler := service.NewAccountsResultHandler(txSvc, logger)

	registerDomainErrors()
	handler := apphttp.NewHandler(txSvc)
	router := apphttp.NewRouter(handler, logger)

	httpSrv := &nethttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	producer := pkgkafka.NewProducer(cfg.KafkaBrokers, logger)
	defer producer.Close()
	dlqProducer := pkgkafka.NewProducer(cfg.KafkaBrokers, logger)
	defer dlqProducer.Close()

	consumer := pkgkafka.NewConsumer(pkgkafka.ConsumerConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.ServiceName,
		Topic:   events.TopicAccountsTxResults,
	}, dlqProducer, logger)
	defer consumer.Close()

	outboxRepo := repo.NewOutboxRepo(pool)
	outboxWorker := outbox.NewWorker(outboxRepo, producer, 500*time.Millisecond, 100, logger)

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
		if err := consumer.Run(ctx, resultHandler.Handle); err != nil {
			logger.Error("consumer stopped with error", "err", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown error", "err", err)
	}

	wg.Wait()
	logger.Info("transactions-ms stopped")
	return nil
}

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

func registerDomainErrors() {
	httpx.RegisterDomainError(domain.ErrTransactionNotFound,
		httpx.NewError(nethttp.StatusNotFound, "transaction_not_found", "La transacción no existe"))
	httpx.RegisterDomainError(domain.ErrInvalidTransactionType,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "invalid_type", "Tipo de transacción inválido"))
	httpx.RegisterDomainError(domain.ErrSameAccountTransfer,
		httpx.NewError(nethttp.StatusUnprocessableEntity, "same_account", "No se puede transferir a la misma cuenta"))
}
