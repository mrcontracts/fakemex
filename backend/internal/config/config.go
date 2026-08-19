package config

import (
	"bufio"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

type Config struct {
	HLNetwork              string
	HLAPIURL               string
	HLWsURL                string
	HLAccountAddress       string
	HLAPIWalletAddress     string
	HLAPIWalletPrivateKey  string
	ServerAddr             string
	FrontendOrigin         string
	LogLevel               string
	TradingAllowed         bool
	AutoBuilderFeeDisabled bool
	AccountConfigured      bool
}

func (c Config) CloneRedacted() map[string]any {
	return map[string]any{
		"HL_NETWORK":                   c.HLNetwork,
		"HL_API_URL":                   c.HLAPIURL,
		"HL_WS_URL":                    c.HLWsURL,
		"HL_ACCOUNT_ADDRESS":           "[REDACTED]",
		"HL_API_WALLET_ADDRESS":        "[REDACTED]",
		"HL_API_WALLET_PRIVATE_KEY":    "[REDACTED]",
		"SERVER_ADDR":                  c.ServerAddr,
		"FRONTEND_ORIGIN":              c.FrontendOrigin,
		"LOG_LEVEL":                    c.LogLevel,
		"TRADING_ENABLED":              c.TradingAllowed,
		"HL_AUTO_BUILDER_FEE_DISABLED": c.AutoBuilderFeeDisabled,
	}
}

func (c Config) HasAccount() bool {
	return c.HLAccountAddress != ""
}

func (c Config) CanTrade() bool {
	return c.TradingAllowed && c.HasAccount() && c.HLAPIWalletAddress != "" && c.HLAPIWalletPrivateKey != ""
}

func Load(path string) (Config, error) {
	if path == "" {
		path = filepath.Join("..", "config", "local.env")
	}
	cfg, err := loadFromFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	cfg.AutoBuilderFeeDisabled = true
	cfg.AccountConfigured = cfg.HasAccount()
	return cfg, nil
}

func loadFromFile(path string) (Config, error) {
	// #nosec G304 -- the path is an explicit local CLI/config choice; supporting
	// alternate credential files is intentional and the file is never remote input.
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("inspect config file %s: %w", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("config file %s must not be accessible by group or others", path)
	}

	scanner := bufio.NewScanner(f)
	vals := make(map[string]string)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, `"'`)
		vals[k] = v
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("scan config file %s: %w", path, err)
	}
	tradingAllowed, err := parseBool(vals["TRADING_ENABLED"])
	if err != nil {
		return Config{}, fmt.Errorf("TRADING_ENABLED: %w", err)
	}
	return Config{
		HLNetwork:             vals["HL_NETWORK"],
		HLAPIURL:              vals["HL_API_URL"],
		HLWsURL:               vals["HL_WS_URL"],
		HLAccountAddress:      vals["HL_ACCOUNT_ADDRESS"],
		HLAPIWalletAddress:    vals["HL_API_WALLET_ADDRESS"],
		HLAPIWalletPrivateKey: vals["HL_API_WALLET_PRIVATE_KEY"],
		ServerAddr:            vals["SERVER_ADDR"],
		FrontendOrigin:        vals["FRONTEND_ORIGIN"],
		LogLevel:              vals["LOG_LEVEL"],
		TradingAllowed:        tradingAllowed,
	}, nil
}

func validate(c Config) error {
	if c.HLNetwork == "" {
		return fmt.Errorf("HL_NETWORK is required")
	}
	if c.HLNetwork != "testnet" {
		return fmt.Errorf("HL_NETWORK must be testnet")
	}
	if c.HLAPIURL == "" {
		return fmt.Errorf("HL_API_URL is required")
	}
	apiURL, err := url.Parse(c.HLAPIURL)
	if err != nil || apiURL.Scheme == "" || apiURL.Hostname() == "" {
		return fmt.Errorf("HL_API_URL must be a valid absolute URL")
	}
	if c.HLWsURL == "" {
		return fmt.Errorf("HL_WS_URL is required")
	}
	if !(strings.HasPrefix(c.HLWsURL, "wss://") || strings.HasPrefix(c.HLWsURL, "ws://")) {
		return fmt.Errorf("HL_WS_URL must be ws:// or wss:// URL")
	}
	if c.ServerAddr == "" {
		return fmt.Errorf("SERVER_ADDR is required")
	}
	if !IsLoopbackAddress(c.ServerAddr) {
		return fmt.Errorf("SERVER_ADDR must bind to loopback")
	}
	if c.FrontendOrigin == "" {
		return fmt.Errorf("FRONTEND_ORIGIN is required")
	}
	if !isLoopbackOrigin(c.FrontendOrigin) {
		return fmt.Errorf("FRONTEND_ORIGIN must be an http(s) loopback origin")
	}
	if c.HLAPIWalletAddress != "" || c.HLAPIWalletPrivateKey != "" {
		addr, err := deriveAddressFromPrivateKey(c.HLAPIWalletPrivateKey)
		if err != nil {
			return fmt.Errorf("HL_API_WALLET_PRIVATE_KEY invalid")
		}
		if c.HLAPIWalletAddress == "" {
			return fmt.Errorf("HL_API_WALLET_ADDRESS is required when private key is provided")
		}
		if !strings.EqualFold(normalizedHex(addr), normalizedHex(c.HLAPIWalletAddress)) {
			return fmt.Errorf("HL_API_WALLET_ADDRESS does not match derived address from HL_API_WALLET_PRIVATE_KEY")
		}
	}
	if c.TradingAllowed {
		if c.HLAccountAddress == "" || c.HLAPIWalletAddress == "" || c.HLAPIWalletPrivateKey == "" {
			return fmt.Errorf("trading requires HL_ACCOUNT_ADDRESS, HL_API_WALLET_ADDRESS, and HL_API_WALLET_PRIVATE_KEY")
		}
		if apiURL.Scheme != "https" || !strings.EqualFold(apiURL.Hostname(), "api.hyperliquid-testnet.xyz") || apiURL.Port() != "" || strings.TrimRight(apiURL.EscapedPath(), "/") != "" {
			return fmt.Errorf("trading requires the official testnet HL_API_URL")
		}
		wsURL, err := url.Parse(c.HLWsURL)
		if err != nil || wsURL.Scheme != "wss" || !strings.EqualFold(wsURL.Hostname(), "api.hyperliquid-testnet.xyz") || wsURL.Port() != "" || strings.TrimRight(wsURL.EscapedPath(), "/") != "/ws" {
			return fmt.Errorf("trading requires the official testnet HL_WS_URL")
		}
	}
	return nil
}

func parseBool(value string) (bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("must be true or false")
	}
	return parsed, nil
}

// IsLoopbackAddress reports whether address is a host:port pair bound only to
// the local machine. It is exported so command-line overrides can be checked
// after the config file has been validated.
func IsLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	if strings.EqualFold(parsed.Hostname(), "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(parsed.Hostname())
	return err == nil && ip.IsLoopback()
}

func deriveAddressFromPrivateKey(privateKey string) (string, error) {
	key := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(privateKey)), "0x")
	if key == "" {
		return "", fmt.Errorf("empty private key")
	}
	bytes, err := crypto.HexToECDSA(key)
	if err != nil {
		return "", err
	}
	return crypto.PubkeyToAddress(bytes.PublicKey).Hex(), nil
}

func normalizedHex(v string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "0x")
}
