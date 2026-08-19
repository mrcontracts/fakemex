package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"fake-mex-backend/internal/config"
	"fake-mex-backend/internal/exchange"
	backendapi "fake-mex-backend/internal/http"
)

func main() {
	addr := flag.String("addr", "", "listen address")
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if *addr != "" {
		if !config.IsLoopbackAddress(*addr) {
			slog.Error("listen address must bind to loopback")
			os.Exit(1)
		}
		cfg.ServerAddr = *addr
	}
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	var client exchange.ExchangeClient
	if cfg.CanTrade() {
		client, err = exchange.NewTradingClient(
			cfg.HLAPIURL,
			cfg.HLWsURL,
			cfg.HLAccountAddress,
			cfg.HLAPIWalletAddress,
			cfg.HLAPIWalletPrivateKey,
			logger,
		)
		if err != nil {
			logger.Error("failed to initialize signed trading", "error", err)
			os.Exit(1)
		}
	} else {
		client = exchange.NewSafeClient(cfg.HLAPIURL, cfg.HLWsURL, cfg.HLAccountAddress, logger)
	}
	// The exchange client retains only the parsed signing key. Avoid keeping a
	// second plaintext copy in the server configuration.
	cfg.HLAPIWalletPrivateKey = ""
	router := backendapi.NewServer(cfg, client, logger).Router()

	srv := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	logger.Info("listening", "addr", cfg.ServerAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}
