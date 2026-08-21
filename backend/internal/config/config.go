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

const (
	NetworkTestnet = "testnet"
	NetworkMainnet = "mainnet"

	TestnetAPIURL = "https://api.hyperliquid-testnet.xyz"
	TestnetWSURL  = "wss://api.hyperliquid-testnet.xyz/ws"
	MainnetAPIURL = "https://api.hyperliquid.xyz"
	MainnetWSURL  = "wss://api.hyperliquid.xyz/ws"
)

type ExchangeConfig struct {
	Network             string
	APIURL              string
	WsURL               string
	AccountAddress      string
	APIWalletAddress    string
	APIWalletPrivateKey string
}

func (c ExchangeConfig) HasAccount() bool {
	return c.AccountAddress != ""
}

func (c ExchangeConfig) HasTradingCredentials() bool {
	return c.HasAccount() && c.APIWalletAddress != "" && c.APIWalletPrivateKey != ""
}

func (c ExchangeConfig) CanTrade(tradingAllowed bool) bool {
	return tradingAllowed && c.HasTradingCredentials()
}

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
	Networks               map[string]ExchangeConfig
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

func (c Config) ForNetwork(network string) (Config, bool) {
	network = strings.ToLower(strings.TrimSpace(network))
	if len(c.Networks) == 0 {
		if strings.EqualFold(c.HLNetwork, network) {
			return c, true
		}
		return Config{}, false
	}
	profile, ok := c.Networks[network]
	if !ok {
		return Config{}, false
	}
	selected := c
	selected.HLNetwork = profile.Network
	selected.HLAPIURL = profile.APIURL
	selected.HLWsURL = profile.WsURL
	selected.HLAccountAddress = profile.AccountAddress
	selected.HLAPIWalletAddress = profile.APIWalletAddress
	selected.HLAPIWalletPrivateKey = profile.APIWalletPrivateKey
	selected.AccountConfigured = profile.HasAccount()
	selected.Networks = nil
	return selected, true
}

func (c Config) ConfiguredNetworks() []string {
	configured := make([]string, 0, 2)
	for _, network := range []string{NetworkTestnet, NetworkMainnet} {
		if _, ok := c.ForNetwork(network); ok {
			configured = append(configured, network)
		}
	}
	return configured
}

func (c Config) WithoutPrivateKeys() Config {
	redacted := c
	redacted.HLAPIWalletPrivateKey = ""
	if len(c.Networks) > 0 {
		redacted.Networks = make(map[string]ExchangeConfig, len(c.Networks))
		for name, profile := range c.Networks {
			profile.APIWalletPrivateKey = ""
			redacted.Networks[name] = profile
		}
	}
	return redacted
}

func (c Config) CanTrade() bool {
	return c.TradingAllowed && c.HasTradingCredentials()
}

func (c Config) HasTradingCredentials() bool {
	return c.HasAccount() && c.HLAPIWalletAddress != "" && c.HLAPIWalletPrivateKey != ""
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
	testnet, _ := cfg.ForNetwork(NetworkTestnet)
	cfg.HLNetwork = testnet.HLNetwork
	cfg.HLAPIURL = testnet.HLAPIURL
	cfg.HLWsURL = testnet.HLWsURL
	cfg.HLAccountAddress = testnet.HLAccountAddress
	cfg.HLAPIWalletAddress = testnet.HLAPIWalletAddress
	cfg.HLAPIWalletPrivateKey = testnet.HLAPIWalletPrivateKey
	cfg.AccountConfigured = testnet.HasAccount()
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
	if err := validateConfigFilePermissions(path, info); err != nil {
		return Config{}, err
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
	testnet := ExchangeConfig{
		Network:             NetworkTestnet,
		APIURL:              firstValue(vals["HL_TESTNET_API_URL"], vals["HL_API_URL"], TestnetAPIURL),
		WsURL:               firstValue(vals["HL_TESTNET_WS_URL"], vals["HL_WS_URL"], TestnetWSURL),
		AccountAddress:      firstValue(vals["HL_TESTNET_ACCOUNT_ADDRESS"], vals["HL_ACCOUNT_ADDRESS"]),
		APIWalletAddress:    firstValue(vals["HL_TESTNET_API_WALLET_ADDRESS"], vals["HL_API_WALLET_ADDRESS"]),
		APIWalletPrivateKey: firstValue(vals["HL_TESTNET_API_WALLET_PRIVATE_KEY"], vals["HL_API_WALLET_PRIVATE_KEY"]),
	}
	mainnet := ExchangeConfig{
		Network:             NetworkMainnet,
		APIURL:              firstValue(vals["HL_MAINNET_API_URL"], MainnetAPIURL),
		WsURL:               firstValue(vals["HL_MAINNET_WS_URL"], MainnetWSURL),
		AccountAddress:      vals["HL_MAINNET_ACCOUNT_ADDRESS"],
		APIWalletAddress:    vals["HL_MAINNET_API_WALLET_ADDRESS"],
		APIWalletPrivateKey: vals["HL_MAINNET_API_WALLET_PRIVATE_KEY"],
	}
	return Config{
		HLNetwork:             firstValue(vals["HL_NETWORK"], NetworkTestnet),
		HLAPIURL:              testnet.APIURL,
		HLWsURL:               testnet.WsURL,
		HLAccountAddress:      testnet.AccountAddress,
		HLAPIWalletAddress:    testnet.APIWalletAddress,
		HLAPIWalletPrivateKey: testnet.APIWalletPrivateKey,
		ServerAddr:            vals["SERVER_ADDR"],
		FrontendOrigin:        vals["FRONTEND_ORIGIN"],
		LogLevel:              vals["LOG_LEVEL"],
		TradingAllowed:        tradingAllowed,
		Networks: map[string]ExchangeConfig{
			NetworkTestnet: testnet,
			NetworkMainnet: mainnet,
		},
	}, nil
}

func validate(c Config) error {
	if legacyNetwork := strings.ToLower(strings.TrimSpace(c.HLNetwork)); legacyNetwork != "" && legacyNetwork != NetworkTestnet {
		return fmt.Errorf("the startup network must be testnet; switch networks at runtime")
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
	canTrade := false
	for _, network := range []string{NetworkTestnet, NetworkMainnet} {
		profile, ok := c.Networks[network]
		if !ok {
			return fmt.Errorf("%s network configuration is required", network)
		}
		if err := validateExchangeConfig(profile); err != nil {
			return err
		}
		canTrade = canTrade || profile.CanTrade(c.TradingAllowed)
	}
	if c.TradingAllowed && !canTrade {
		return fmt.Errorf("TRADING_ENABLED requires complete API-wallet credentials for at least one network")
	}
	return nil
}

func validateExchangeConfig(c ExchangeConfig) error {
	expectedAPIURL, expectedWSURL := TestnetAPIURL, TestnetWSURL
	if c.Network == NetworkMainnet {
		expectedAPIURL, expectedWSURL = MainnetAPIURL, MainnetWSURL
	} else if c.Network != NetworkTestnet {
		return fmt.Errorf("unsupported Hyperliquid network %q", c.Network)
	}
	if !sameOfficialURL(c.APIURL, expectedAPIURL) {
		return fmt.Errorf("HL_%s_API_URL must be the official %s endpoint", strings.ToUpper(c.Network), c.Network)
	}
	if !sameOfficialURL(c.WsURL, expectedWSURL) {
		return fmt.Errorf("HL_%s_WS_URL must be the official %s endpoint", strings.ToUpper(c.Network), c.Network)
	}
	if c.APIWalletAddress != "" || c.APIWalletPrivateKey != "" {
		if c.AccountAddress == "" {
			return fmt.Errorf("HL_%s_ACCOUNT_ADDRESS is required with API-wallet credentials", strings.ToUpper(c.Network))
		}
		address, err := deriveAddressFromPrivateKey(c.APIWalletPrivateKey)
		if err != nil {
			return fmt.Errorf("HL_%s_API_WALLET_PRIVATE_KEY invalid", strings.ToUpper(c.Network))
		}
		if c.APIWalletAddress == "" {
			return fmt.Errorf("HL_%s_API_WALLET_ADDRESS is required when private key is provided", strings.ToUpper(c.Network))
		}
		if !strings.EqualFold(normalizedHex(address), normalizedHex(c.APIWalletAddress)) {
			return fmt.Errorf("HL_%s_API_WALLET_ADDRESS does not match its private key", strings.ToUpper(c.Network))
		}
	}
	return nil
}

func sameOfficialURL(value, expected string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	expectedURL, _ := url.Parse(expected)
	return parsed.Scheme == expectedURL.Scheme &&
		strings.EqualFold(parsed.Hostname(), expectedURL.Hostname()) &&
		parsed.Port() == "" &&
		strings.TrimRight(parsed.EscapedPath(), "/") == strings.TrimRight(expectedURL.EscapedPath(), "/")
}

func firstValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
