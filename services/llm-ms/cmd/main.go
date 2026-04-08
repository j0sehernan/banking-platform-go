// llm-ms expone explicaciones (cacheadas) y un chat scoped a una
// transacción específica usando context grounding y SSE streaming.
//
// Mantiene una vista materializada de transacciones (`transactions_view`)
// alimentada por eventos de Kafka — así no depende síncronamente de
// transactions-ms para responder preguntas sobre una transacción.
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
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/config"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/domain"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/explainer"
	apphttp "github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/http"
	"github.com/j0sehernan/banking-platform-go/services/llm-ms/internal/service"
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

	// Decidir explainer: real o mock
	var exp explainer.Explainer
	if cfg.AnthropicAPIKey == "" {
		logger.Warn("ANTHROPIC_API_KEY no configurada, usando MockExplainer")
		exp = explainer.NewMockExplainer()
	} else {
		logger.Info("usando ClaudeExplainer", "model", cfg.LLMModel)
		exp = explainer.NewClaudeExplainer(cfg.AnthropicAPIKey, cfg.LLMModel)
	}

	logger.Info("starting llm-ms",
		"http_addr", cfg.HTTPAddr,
		"kafka_brokers", cfg.KafkaBrokers,
		"explainer", exp.Model(),
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

	svc := service.NewLLMService(pool, exp)
	eventHandler := service.NewEventHandler(pool, svc, logger)

	registerDomainErrors()
	handler := apphttp.NewHandler(svc)
	router := apphttp.NewRouter(handler, logger)

	httpSrv := &nethttp.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		// timeouts grandes para SSE de chat (puede tardar varios segundos)
		WriteTimeout: 5 * time.Minute,
	}

	dlqProducer := pkgkafka.NewProducer(cfg.KafkaBrokers, logger)
	defer dlqProducer.Close()

	consumer := pkgkafka.NewConsumer(pkgkafka.ConsumerConfig{
		Brokers: cfg.KafkaBrokers,
		GroupID: cfg.ServiceName,
		Topic:   events.TopicTransactionsEvents,
	}, dlqProducer, logger)
	defer consumer.Close()

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
		if err := consumer.Run(ctx, eventHandler.Handle); err != nil {
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
	logger.Info("llm-ms stopped")
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
		httpx.NewError(nethttp.StatusNotFound, "transaction_not_found", "La transacción no existe en la vista"))
	httpx.RegisterDomainError(domain.ErrExplanationNotFound,
		httpx.NewError(nethttp.StatusNotFound, "explanation_not_found", "Aún no hay explicación para esta transacción"))
}
