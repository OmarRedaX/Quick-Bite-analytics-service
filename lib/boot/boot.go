// Package boot wires every singleton this service needs — the Go analogue
// of order-service's lib/di/container.ts, minus a DI framework. Adding a
// module means editing Run() once; cmd/api/main.go never changes.
package boot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	mongodriver "go.mongodb.org/mongo-driver/mongo"

	"analytics-service/app/analytics/controller"
	"analytics-service/app/analytics/eventhandlers"
	"analytics-service/app/analytics/repository"
	"analytics-service/app/analytics/service"
	"analytics-service/lib/config"
	"analytics-service/lib/coreclient"
	"analytics-service/lib/coreevents"
	response "analytics-service/lib/http"
	"analytics-service/lib/logger"
	"analytics-service/lib/middleware"
	"analytics-service/lib/rbac"
	"analytics-service/pkg/httpclient"
	"analytics-service/pkg/messaging"
	appmongo "analytics-service/pkg/mongo"
)

// Run loads config, connects every dependency, mounts routes, starts the
// event consumer, and blocks serving HTTP until SIGINT/SIGTERM. This is the
// entire body of cmd/api/main.go's call.
func Run() error {
	cfg := config.Load()
	log := logger.New(slog.LevelInfo)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Mongo ────────────────────────────────────────────────────────────
	mongoClient, db, err := connectMongoWithRetry(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("boot: mongo: %w", err)
	}
	log.Info("mongo connected", "database", cfg.Mongo.Database)

	if err := repository.EnsureIndexes(ctx, db, cfg.EventDedupeTTL, cfg.OrderContextTTL); err != nil {
		return fmt.Errorf("boot: ensure indexes: %w", err)
	}
	log.Info("mongo indexes ensured")

	// ── Repositories, service, controller ───────────────────────────────
	restaurantDayRepo := repository.NewRestaurantDayRepo(db)
	branchDayRepo := repository.NewBranchDayRepo(db)
	productDayRepo := repository.NewProductDayRepo(db)
	platformDayRepo := repository.NewPlatformDayRepo(db)
	orderContextRepo := repository.NewOrderContextRepo(db)
	eventIDsRepo := repository.NewEventIDsRepo(db)

	analyticsService := service.New(restaurantDayRepo, branchDayRepo, productDayRepo, platformDayRepo, orderContextRepo)
	analyticsController := controller.New(analyticsService)

	// ── core-service client + RBAC cache ────────────────────────────────
	httpClient := httpclient.New(httpclient.Config{Timeout: cfg.Core.HTTPTimeout, MaxRetries: 2})
	coreClient := coreclient.New(httpClient, cfg.Core.BaseURL, cfg.Core.InternalAPIKey)
	permCache := rbac.NewCache(cfg.RBAC.CacheTTL, coreClient.GetPermissionsByRole)

	// ── HTTP router ──────────────────────────────────────────────────────
	router := chi.NewRouter()
	router.Use(middleware.Correlation(log))
	router.Use(middleware.AccessLog)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response.SendSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Mount("/api/v1/analytics", controller.Routes(cfg.JWT.AccessSecret, permCache, analyticsController))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	// ── RabbitMQ consumer ────────────────────────────────────────────────
	broker := messaging.NewAMQPClient(messaging.AMQPConfig{URL: cfg.Rabbit.URL})
	if err := connectBrokerWithRetry(ctx, broker, log); err != nil {
		return fmt.Errorf("boot: rabbit: %w", err)
	}
	log.Info("rabbit connected")

	consumer := coreevents.NewConsumer(broker, messaging.ConsumerOptions{
		Exchange:           cfg.Rabbit.OrderEventsExchange,
		Queue:              cfg.Rabbit.Queue,
		BindingKeys:        cfg.Rabbit.Bindings,
		DeadLetterExchange: cfg.Rabbit.DLX,
		DeadLetterQueue:    cfg.Rabbit.DLQ,
		Prefetch:           cfg.Rabbit.Prefetch,
	}, eventIDsRepo, log)

	eventhandlers.Register(consumer, analyticsService)

	if err := consumer.Start(ctx); err != nil {
		return fmt.Errorf("boot: start consumer: %w", err)
	}

	// core.events is a separate exchange from order.events above — a second
	// consumer/queue/binding, not a new routing key on the existing one (same
	// broker connection and event_ids dedupe collection are safe to share:
	// eventIds are unique regardless of source exchange).
	coreEventsConsumer := coreevents.NewConsumer(broker, messaging.ConsumerOptions{
		Exchange:           cfg.Rabbit.CoreEventsExchange,
		Queue:              cfg.Rabbit.CoreEventsQueue,
		BindingKeys:        cfg.Rabbit.CoreEventsBindings,
		DeadLetterExchange: cfg.Rabbit.CoreEventsDLX,
		DeadLetterQueue:    cfg.Rabbit.CoreEventsDLQ,
		Prefetch:           cfg.Rabbit.Prefetch,
	}, eventIDsRepo, log)

	coreEventsConsumer.Register("rbac.permissions_changed", permCache.HandlePermissionsChanged(log))

	if err := coreEventsConsumer.Start(ctx); err != nil {
		return fmt.Errorf("boot: start core-events consumer: %w", err)
	}

	// ── Serve ────────────────────────────────────────────────────────────
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown requested")
	case err := <-serveErr:
		log.Error("http server failed", "error", err.Error())
	}

	return shutdown(server, broker, mongoClient, log)
}

func shutdown(server *http.Server, broker *messaging.AMQPClient, mongoClient *mongodriver.Client, log *slog.Logger) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown error", "error", err.Error())
	}
	if err := broker.Close(); err != nil {
		log.Warn("broker close error", "error", err.Error())
	}
	if err := appmongo.Disconnect(shutdownCtx, mongoClient); err != nil {
		log.Warn("mongo disconnect error", "error", err.Error())
	}
	return nil
}

func connectMongoWithRetry(ctx context.Context, cfg *config.Config, log *slog.Logger) (*mongodriver.Client, *mongodriver.Database, error) {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		client, db, err := appmongo.Connect(ctx, appmongo.Config{
			URI:            cfg.Mongo.URI,
			Database:       cfg.Mongo.Database,
			ConnectTimeout: cfg.Mongo.ConnectTimeout,
		})
		if err == nil {
			return client, db, nil
		}
		lastErr = err
		log.Warn("mongo connect failed, retrying", "attempt", attempt, "error", err.Error())
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return nil, nil, lastErr
}

func connectBrokerWithRetry(ctx context.Context, broker *messaging.AMQPClient, log *slog.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := broker.Connect(ctx); err == nil {
			return nil
		} else {
			lastErr = err
			log.Warn("rabbit connect failed, retrying", "attempt", attempt, "error", err.Error())
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}
