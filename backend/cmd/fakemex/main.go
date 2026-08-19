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

	bindings := make(map[string]backendapi.NetworkBinding, len(cfg.ConfiguredNetworks()))
	for _, network := range cfg.ConfiguredNetworks() {
		profile, ok := cfg.ForNetwork(network)
		if !ok {
			logger.Error("network configuration disappeared", "network", network)
			os.Exit(1)
		}

		var client exchange.ExchangeClient
		configured := profile.HasTradingCredentials()
		tradingAvailable := profile.CanTrade()
		if tradingAvailable {
			client, err = exchange.NewTradingClient(
				profile.HLAPIURL,
				profile.HLWsURL,
				profile.HLAccountAddress,
				profile.HLAPIWalletAddress,
				profile.HLAPIWalletPrivateKey,
				network == config.NetworkMainnet,
				logger,
			)
			if err != nil {
				logger.Error("failed to initialize signed trading", "network", network, "error", err)
				os.Exit(1)
			}
		} else {
			client = exchange.NewSafeClient(profile.HLAPIURL, profile.HLWsURL, profile.HLAccountAddress, logger)
		}
		// Each client retains only its parsed signing key. The HTTP runtime gets
		// network metadata but no plaintext private-key copy.
		profile.HLAPIWalletPrivateKey = ""
		bindings[network] = backendapi.NetworkBinding{
			Config:           profile,
			Client:           client,
			Configured:       configured,
			TradingAvailable: tradingAvailable,
		}
	}
	cfg = cfg.WithoutPrivateKeys()
	server, err := backendapi.NewNetworkServer(cfg, bindings, config.NetworkTestnet, logger)
	if err != nil {
		logger.Error("failed to initialize network runtime", "error", err)
		os.Exit(1)
	}
	router := server.Router()

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
