package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"atm-identity-api/internal/apisperu"
	"atm-identity-api/internal/companies"
	"atm-identity-api/internal/persons"
	"atm-identity-api/internal/platform/apiresponse"
	"atm-identity-api/internal/platform/auth"
	"atm-identity-api/internal/platform/config"
	"atm-identity-api/internal/platform/database"
	"atm-identity-api/internal/platform/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		return healthcheck()
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "migrate":
		return database.Migrate(ctx, pool)
	case "create-client-system": // usage: create-client-system <code> <name>
		if len(os.Args) != 4 {
			return errors.New("usage: server create-client-system <code> <name>")
		}
		key, err := auth.CreateClientSystem(ctx, pool, os.Args[2], os.Args[3])
		if err != nil {
			return err
		}
		fmt.Printf("API key for %q (shown once, store it now):\n%s\n", os.Args[2], key)
		return nil
	case "serve":
		if err := database.Migrate(ctx, pool); err != nil {
			return err
		}
		return serve(ctx, cfg, pool)
	default:
		return fmt.Errorf("unknown command %q (serve, migrate, create-client-system, healthcheck)", cmd)
	}
}

// healthcheck probes the local server; used by the Docker HEALTHCHECK because
// the final image has no curl.
func healthcheck() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	return nil
}

func serve(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	upstream := apisperu.NewClient(cfg.APIsPeruBaseURL, cfg.APIsPeruToken, cfg.APIsPeruTimeout)

	router := gin.New()
	router.Use(middleware.RequestID(), middleware.Logger(), middleware.Recovery())
	router.NoRoute(middleware.NotFound)

	router.GET("/health", func(c *gin.Context) {
		apiresponse.OK(c, gin.H{"status": "up"}, "")
	})

	v1 := router.Group("/v1", auth.Middleware(pool))
	persons.NewHandler(persons.NewService(persons.NewRepository(pool), upstream)).Register(v1)
	companies.NewHandler(companies.NewService(companies.NewRepository(pool), upstream)).Register(v1)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	slog.Info("server listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
