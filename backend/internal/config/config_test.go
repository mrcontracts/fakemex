package config_test

import (
	"crypto/ecdsa"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fake-mex-backend/internal/config"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestLoadConfigSuccessAndAddressValidation(t *testing.T) {
	t.Parallel()

	key, walletAddress := mustCreateSignedWallet(t)

	content := strings.Join([]string{
		"HL_NETWORK=testnet",
		"HL_API_URL=https://api.hyperliquid-testnet.xyz",
		"HL_WS_URL=wss://api.hyperliquid-testnet.xyz/ws",
		"HL_ACCOUNT_ADDRESS=0xfeed",
		"HL_API_WALLET_ADDRESS=" + walletAddress,
		"HL_API_WALLET_PRIVATE_KEY=" + key,
		"SERVER_ADDR=127.0.0.1:8080",
		"FRONTEND_ORIGIN=http://localhost:4200",
		"LOG_LEVEL=info",
		"TRADING_ENABLED=true",
	}, "\n")

	cfgPath := writeTempConfig(t, content)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.HLAPIWalletAddress != walletAddress {
		t.Fatalf("wallet address mismatch: got %s", cfg.HLAPIWalletAddress)
	}
	if !cfg.AccountConfigured {
		t.Fatal("expected AccountConfigured=true")
	}
}

func TestLoadConfigMissingNetwork(t *testing.T) {
	t.Parallel()

	cfgPath := writeTempConfig(t, strings.Join([]string{
		"HL_API_URL=https://api.hyperliquid-testnet.xyz",
		"HL_WS_URL=wss://api.hyperliquid-testnet.xyz/ws",
		"SERVER_ADDR=127.0.0.1:8080",
		"FRONTEND_ORIGIN=http://localhost:4200",
	}, "\n"))
	if _, err := config.Load(cfgPath); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigWalletAddressMismatch(t *testing.T) {
	t.Parallel()

	key, _ := mustCreateSignedWallet(t)

	cfgPath := writeTempConfig(t, strings.Join([]string{
		"HL_NETWORK=testnet",
		"HL_API_URL=https://api.hyperliquid-testnet.xyz",
		"HL_WS_URL=wss://api.hyperliquid-testnet.xyz/ws",
		"HL_ACCOUNT_ADDRESS=0xfeed",
		"HL_API_WALLET_ADDRESS=0xdeadbeef",
		"HL_API_WALLET_PRIVATE_KEY=" + key,
		"SERVER_ADDR=127.0.0.1:8080",
		"FRONTEND_ORIGIN=http://localhost:4200",
	}, "\n"))

	_, err := config.Load(cfgPath)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "does not match") {
		t.Fatalf("expected wallet mismatch error, got: %v", err)
	}
}

func TestCloneRedactedHidesPrivateKey(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		HLNetwork:              "testnet",
		HLAPIURL:               "https://api.hyperliquid-testnet.xyz",
		HLWsURL:                "wss://api.hyperliquid-testnet.xyz/ws",
		HLAccountAddress:       "0x123",
		HLAPIWalletAddress:     "0xabc",
		HLAPIWalletPrivateKey:  "0xsecret",
		ServerAddr:             "127.0.0.1:8080",
		FrontendOrigin:         "http://localhost:4200",
		LogLevel:               "info",
		AutoBuilderFeeDisabled: true,
	}
	redacted := cfg.CloneRedacted()
	if redacted["HL_API_WALLET_PRIVATE_KEY"] != "[REDACTED]" {
		t.Fatalf("expected redacted private key, got: %q", redacted["HL_API_WALLET_PRIVATE_KEY"])
	}
	if redacted["HL_API_WALLET_ADDRESS"] != "[REDACTED]" || redacted["HL_ACCOUNT_ADDRESS"] != "[REDACTED]" {
		t.Fatal("expected account addresses to be redacted")
	}
}

func TestHasAccount(t *testing.T) {
	t.Parallel()

	cfg := config.Config{HLAccountAddress: "0xacc", HLAPIWalletAddress: "0xwallet", HLAPIWalletPrivateKey: "0xkey"}
	if !cfg.HasAccount() {
		t.Fatal("expected account configured")
	}

	cfg.HLAPIWalletPrivateKey = ""
	if !cfg.HasAccount() {
		t.Fatal("account reads only require the account address")
	}
	if cfg.CanTrade() {
		t.Fatal("trading must require explicit enablement and wallet credentials")
	}
}

func TestTradingRequiresWalletCredentialsAndOfficialEndpoints(t *testing.T) {
	t.Parallel()

	cfgPath := writeTempConfig(t, strings.Join([]string{
		"HL_NETWORK=testnet",
		"HL_API_URL=https://example.test",
		"HL_WS_URL=wss://example.test/ws",
		"HL_ACCOUNT_ADDRESS=0xfeed",
		"SERVER_ADDR=127.0.0.1:8080",
		"FRONTEND_ORIGIN=http://localhost:4200",
		"TRADING_ENABLED=true",
	}, "\n"))
	if _, err := config.Load(cfgPath); err == nil || !strings.Contains(err.Error(), "trading requires") {
		t.Fatalf("expected trading configuration error, got %v", err)
	}
}

func TestServerMustBindLoopback(t *testing.T) {
	t.Parallel()

	cfgPath := writeTempConfig(t, strings.Join([]string{
		"HL_NETWORK=testnet",
		"HL_API_URL=https://api.hyperliquid-testnet.xyz",
		"HL_WS_URL=wss://api.hyperliquid-testnet.xyz/ws",
		"SERVER_ADDR=0.0.0.0:8080",
		"FRONTEND_ORIGIN=http://localhost:4200",
	}, "\n"))
	if _, err := config.Load(cfgPath); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback validation error, got %v", err)
	}
}

func mustCreateSignedWallet(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	key := hexutil.Encode(crypto.FromECDSA(privateKey))
	return key, strings.ToLower(address)
}

func writeTempConfig(t *testing.T, lines string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "local.env")
	if err := os.WriteFile(cfgPath, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}
