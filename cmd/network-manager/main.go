// network-manager is the Fleet Orchestration component that manages all network
// configuration for the Technicolor Haze platform.
//
// It stores tenant network state (VPCs, firewall rules, NAT, rate limits) and
// serves the desired state to DPU agents via a pull model.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mara-baysn/technicolor-haze-network-manager/internal/ipam"
	"github.com/mara-baysn/technicolor-haze-network-manager/internal/server"
	"github.com/mara-baysn/technicolor-haze-network-manager/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Configuration from environment
	port := envOr("PORT", "8080")
	logLevel := envOr("LOG_LEVEL", "info")
	ipPoolCIDR := envOr("IP_POOL_CIDR", "203.0.113.0/24") // Default: documentation range

	// Setup logger
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	// Initialize store (in-memory for v1)
	memStore := store.NewMemoryStore()
	logger.Info("initialized in-memory store")

	// Initialize IP pool
	pool := ipam.NewPool()
	count, err := pool.AddCIDR(ipPoolCIDR)
	if err != nil {
		return fmt.Errorf("failed to initialize IP pool from %s: %w", ipPoolCIDR, err)
	}
	logger.Info("initialized IP pool", "cidr", ipPoolCIDR, "available_ips", count)

	// Create server
	srv := server.New(memStore, pool, logger)

	// HTTP server
	httpSrv := &http.Server{
		Addr:         ":" + port,
		Handler:      srv.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server
	go func() {
		logger.Info("starting network manager", "port", port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	logger.Info("shutdown complete")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
