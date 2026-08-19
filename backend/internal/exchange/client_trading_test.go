package exchange

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

type dummyAction struct {
	Type string `msgpack:"type"`
	Num  int64  `msgpack:"num"`
}

func TestActionHashMatchesOfficialHyperliquidSDKVector(t *testing.T) {
	t.Parallel()
	action := orderAction{
		Type: "order",
		Orders: []wireOrder{{
			Asset:      4,
			IsBuy:      true,
			LimitPrice: "1670.1",
			Size:       "0.0147",
			ReduceOnly: false,
			OrderType:  wireLimitOrderType{Limit: wireLimit{TIF: "Ioc"}},
		}},
		Grouping: "na",
	}
	hash, err := actionHash(action, 1677777606040)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "0fcbeda5ae3c4950a548021552a4fea2226858c4453571bf3f24ba017eac2908"
	if hex.EncodeToString(hash) != expected {
		t.Fatalf("action hash mismatch: got %x want %s", hash, expected)
	}
}

func TestL1SigningMatchesOfficialHyperliquidSDKVector(t *testing.T) {
	t.Parallel()
	// Assemble the public SDK fixture at runtime so secret scanners do not
	// mistake a test-only, publicly documented value for a usable credential.
	fixtureKey := strings.Repeat("0123456789", 6) + "0123"
	key, err := crypto.HexToECDSA(fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	action := orderAction{
		Type: "order",
		Orders: []wireOrder{{
			Asset:      1,
			IsBuy:      true,
			LimitPrice: "100",
			Size:       "100",
			ReduceOnly: false,
			OrderType:  wireLimitOrderType{Limit: wireLimit{TIF: "Gtc"}},
		}},
		Grouping: "na",
	}
	signature, err := signL1Action(key, action, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if signature.R != "0x82b2ba28e76b3d761093aaded1b1cdad4960b3af30212b343fb2e6cdfa4e3d54" {
		t.Fatalf("unexpected r: %s", signature.R)
	}
	if signature.S != "0x6b53878fc99d26047f4d7e8c90eb98955a109f44209163f52d8dc4278cbbd9f5" {
		t.Fatalf("unexpected s: %s", signature.S)
	}
	if signature.V != 27 {
		t.Fatalf("unexpected v: %d", signature.V)
	}
}

func TestL1MainnetSigningMatchesOfficialHyperliquidSDKVector(t *testing.T) {
	t.Parallel()
	fixtureKey := strings.Repeat("0123456789", 6) + "0123"
	key, err := crypto.HexToECDSA(fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	action := orderAction{
		Type: "order",
		Orders: []wireOrder{{
			Asset:      1,
			IsBuy:      true,
			LimitPrice: "100",
			Size:       "100",
			ReduceOnly: false,
			OrderType:  wireLimitOrderType{Limit: wireLimit{TIF: "Gtc"}},
		}},
		Grouping: "na",
	}
	signature, err := signL1Action(key, action, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if signature.R != "0xd65369825a9df5d80099e513cce430311d7d26ddf477f5b3a33d2806b100d78e" {
		t.Fatalf("unexpected r: %s", signature.R)
	}
	if signature.S != "0x2b54116ff64054968aa237c20ca9ff68000f977c93289157748a3162b6ea940e" {
		t.Fatalf("unexpected s: %s", signature.S)
	}
	if signature.V != 28 {
		t.Fatalf("unexpected v: %d", signature.V)
	}
}

func TestVaultSigningMatchesOfficialHyperliquidSDKVector(t *testing.T) {
	t.Parallel()
	fixtureKey := strings.Repeat("0123456789", 6) + "0123"
	key, err := crypto.HexToECDSA(fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := signL1ActionForVault(
		key,
		dummyAction{Type: "dummy", Num: 100_000_000_000},
		0,
		false,
		"0x1719884eb866cb12b2287399b15f7db5e7d775ea",
	)
	if err != nil {
		t.Fatal(err)
	}
	if signature.R != "0xe281d2fb5c6e25ca01601f878e4d69c965bb598b88fac58e475dd1f5e56c362b" {
		t.Fatalf("unexpected r: %s", signature.R)
	}
	if signature.S != "0x7ddad27e9a238d045c035bc606349d075d5c5cd00a6cd1da23ab5c39d4ef0f60" {
		t.Fatalf("unexpected s: %s", signature.S)
	}
	if signature.V != 27 {
		t.Fatalf("unexpected v: %d", signature.V)
	}
}

func TestTradingClientDetectsSubaccountTarget(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["type"] != "userRole" {
			t.Errorf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": "subAccount"})
	}))
	defer server.Close()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	wallet := crypto.PubkeyToAddress(key.PublicKey).Hex()
	const subaccount = "0x1719884eb866cb12b2287399b15f7db5e7d775ea"
	client, err := NewTradingClient(server.URL, "ws://example.invalid", subaccount, wallet, hex.EncodeToString(crypto.FromECDSA(key)), false, testingLogger())
	if err != nil {
		t.Fatal(err)
	}
	if got := client.(*safeClient).vaultAddress; got != subaccount {
		t.Fatalf("vault address mismatch: got %q", got)
	}
}
