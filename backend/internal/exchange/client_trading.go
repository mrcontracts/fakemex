package exchange

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	defaultMarketSlippagePercent = 0.5
	maxExchangeResponseBytes     = 4 << 20
)

var errWritesDisabled = errors.New("signed trading is unavailable")

type wireLimit struct {
	TIF string `json:"tif" msgpack:"tif"`
}

type wireLimitOrderType struct {
	Limit wireLimit `json:"limit" msgpack:"limit"`
}

type wireTrigger struct {
	IsMarket  bool   `json:"isMarket" msgpack:"isMarket"`
	TriggerPx string `json:"triggerPx" msgpack:"triggerPx"`
	TPSL      string `json:"tpsl" msgpack:"tpsl"`
}

type wireTriggerOrderType struct {
	Trigger wireTrigger `json:"trigger" msgpack:"trigger"`
}

type wireOrder struct {
	Asset         int    `json:"a" msgpack:"a"`
	IsBuy         bool   `json:"b" msgpack:"b"`
	LimitPrice    string `json:"p" msgpack:"p"`
	Size          string `json:"s" msgpack:"s"`
	ReduceOnly    bool   `json:"r" msgpack:"r"`
	OrderType     any    `json:"t" msgpack:"t"`
	ClientOrderID string `json:"c,omitempty" msgpack:"c,omitempty"`
}

type orderAction struct {
	Type     string      `json:"type" msgpack:"type"`
	Orders   []wireOrder `json:"orders" msgpack:"orders"`
	Grouping string      `json:"grouping" msgpack:"grouping"`
}

type modifyWire struct {
	OID   any       `json:"oid" msgpack:"oid"`
	Order wireOrder `json:"order" msgpack:"order"`
}

type modifyAction struct {
	Type     string       `json:"type" msgpack:"type"`
	Modifies []modifyWire `json:"modifies" msgpack:"modifies"`
}

type cancelWire struct {
	Asset int   `json:"a" msgpack:"a"`
	OID   int64 `json:"o" msgpack:"o"`
}

type cancelAction struct {
	Type    string       `json:"type" msgpack:"type"`
	Cancels []cancelWire `json:"cancels" msgpack:"cancels"`
}

type leverageAction struct {
	Type     string `json:"type" msgpack:"type"`
	Asset    int    `json:"asset" msgpack:"asset"`
	IsCross  bool   `json:"isCross" msgpack:"isCross"`
	Leverage int    `json:"leverage" msgpack:"leverage"`
}

type exchangeSignature struct {
	R string `json:"r"`
	S string `json:"s"`
	V int    `json:"v"`
}

type exchangeEnvelope struct {
	Action       any               `json:"action"`
	Nonce        int64             `json:"nonce"`
	Signature    exchangeSignature `json:"signature"`
	VaultAddress any               `json:"vaultAddress"`
	ExpiresAfter any               `json:"expiresAfter"`
}

func NewTradingClient(apiURL, wsURL, accountAddress, walletAddress, privateKey string, logger *slog.Logger) (ExchangeClient, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(privateKey), "0x"))
	if err != nil {
		return nil, fmt.Errorf("create signing wallet: invalid private key")
	}
	derived := crypto.PubkeyToAddress(key.PublicKey).Hex()
	if !strings.EqualFold(derived, strings.TrimSpace(walletAddress)) {
		return nil, fmt.Errorf("create signing wallet: wallet address mismatch")
	}
	client := newSafeClient(apiURL, wsURL, accountAddress, logger)
	client.signingKey = key
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	vaultAddress, err := client.resolveTradingTarget(ctx, accountAddress)
	if err != nil {
		return nil, fmt.Errorf("validate trading account: %w", err)
	}
	client.vaultAddress = vaultAddress
	return client, nil
}

func (c *safeClient) resolveTradingTarget(ctx context.Context, accountAddress string) (string, error) {
	var response map[string]any
	if err := c.postJSON(ctx, "/info", map[string]any{"type": "userRole", "user": accountAddress}, &response); err != nil {
		return "", err
	}
	switch asString(response["role"]) {
	case "user":
		return "", nil
	case "subAccount", "vault":
		if !isHexAddress(accountAddress) {
			return "", errors.New("subaccount or vault address is invalid")
		}
		return strings.ToLower(accountAddress), nil
	case "agent":
		return "", errors.New("HL_ACCOUNT_ADDRESS must be the master/subaccount address, not the API wallet address")
	case "missing":
		return "", errors.New("HL_ACCOUNT_ADDRESS does not exist on testnet")
	default:
		return "", errors.New("exchange returned an unknown account role")
	}
}

func (c *safeClient) PlaceOrder(ctx context.Context, request OrderRequest) (WriteResponse, error) {
	market, err := c.marketForSymbol(ctx, request.Symbol)
	if err != nil {
		return writeFailure(err)
	}
	order, err := c.orderWireForRequest(request, market)
	if err != nil {
		return writeFailure(err)
	}
	orders := []wireOrder{order}
	grouping := "na"
	if request.AttachedTakeProfit != nil || request.AttachedStopLoss != nil {
		if request.Kind != "limit" && request.Kind != "market" {
			return writeFailure(errors.New("attached TP/SL is only supported on limit or market parent orders"))
		}
		childSide := "sell"
		if strings.EqualFold(request.Side, "sell") {
			childSide = "buy"
		}
		if request.AttachedTakeProfit != nil {
			child, buildErr := c.attachedOrderWire(market, childSide, request.Size, "tp", *request.AttachedTakeProfit, request.SlippagePercent)
			if buildErr != nil {
				return writeFailure(buildErr)
			}
			orders = append(orders, child)
		}
		if request.AttachedStopLoss != nil {
			child, buildErr := c.attachedOrderWire(market, childSide, request.Size, "sl", *request.AttachedStopLoss, request.SlippagePercent)
			if buildErr != nil {
				return writeFailure(buildErr)
			}
			orders = append(orders, child)
		}
		grouping = "normalTpsl"
	}
	return c.signAndPost(ctx, orderAction{Type: "order", Orders: orders, Grouping: grouping})
}

func (c *safeClient) ModifyOrder(ctx context.Context, oid string, request ModifyOrderRequest) (WriteResponse, error) {
	market, err := c.marketForSymbol(ctx, request.Symbol)
	if err != nil {
		return writeFailure(err)
	}
	parsedOID, err := parseOIDOrCLOID(oid)
	if err != nil {
		return writeFailure(err)
	}
	order, err := buildLimitOrderWire(market.AssetIndex, request.Side, request.Size, request.Price, request.TimeInForce, request.ReduceOnly, "")
	if err != nil {
		return writeFailure(err)
	}
	action := modifyAction{Type: "batchModify", Modifies: []modifyWire{{OID: parsedOID, Order: order}}}
	return c.signAndPost(ctx, action)
}

func (c *safeClient) CancelOrder(ctx context.Context, _ string, symbol, oid string) (WriteResponse, error) {
	market, err := c.marketForSymbol(ctx, symbol)
	if err != nil {
		return writeFailure(err)
	}
	parsedOID, err := strconv.ParseInt(strings.TrimSpace(oid), 10, 64)
	if err != nil || parsedOID <= 0 {
		return writeFailure(errors.New("order id must be a positive integer"))
	}
	return c.signAndPost(ctx, cancelAction{Type: "cancel", Cancels: []cancelWire{{Asset: market.AssetIndex, OID: parsedOID}}})
}

func (c *safeClient) CancelAllOrders(ctx context.Context, address, symbol string) (WriteResponse, error) {
	marketBySymbol := make(map[string]*Market)
	markets, err := c.Markets(ctx)
	if err != nil {
		return writeFailure(err)
	}
	for i := range markets {
		market := markets[i]
		marketBySymbol[strings.ToUpper(market.Symbol)] = &market
	}
	openOrders, err := c.Orders(ctx, address, "open")
	if err != nil {
		return writeFailure(err)
	}
	triggerOrders, err := c.Orders(ctx, address, "trigger")
	if err != nil {
		return writeFailure(err)
	}
	wires := make([]cancelWire, 0, len(openOrders)+len(triggerOrders))
	seen := make(map[string]struct{})
	requestedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	for _, order := range append(openOrders, triggerOrders...) {
		orderSymbol := strings.ToUpper(strings.TrimSpace(order.Symbol))
		if requestedSymbol != "" && orderSymbol != requestedSymbol {
			continue
		}
		if _, ok := seen[order.Oid]; ok {
			continue
		}
		market := marketBySymbol[orderSymbol]
		if market == nil {
			return writeFailure(fmt.Errorf("unknown market for open order %s", order.Oid))
		}
		oid, parseErr := strconv.ParseInt(order.Oid, 10, 64)
		if parseErr != nil || oid <= 0 {
			return writeFailure(fmt.Errorf("invalid open order id %q", order.Oid))
		}
		seen[order.Oid] = struct{}{}
		wires = append(wires, cancelWire{Asset: market.AssetIndex, OID: oid})
	}
	if len(wires) == 0 {
		return WriteResponse{Status: "ok", Message: "no open orders"}, nil
	}
	return c.signAndPost(ctx, cancelAction{Type: "cancel", Cancels: wires})
}

func (c *safeClient) SetLeverage(ctx context.Context, _ string, symbol string, request LeverageRequest) (WriteResponse, error) {
	market, err := c.marketForSymbol(ctx, symbol)
	if err != nil {
		return writeFailure(err)
	}
	if request.Leverage > market.MaxLeverage {
		return writeFailure(fmt.Errorf("leverage exceeds market maximum of %d", market.MaxLeverage))
	}
	action := leverageAction{Type: "updateLeverage", Asset: market.AssetIndex, IsCross: request.Mode == "cross", Leverage: request.Leverage}
	return c.signAndPost(ctx, action)
}

func (c *safeClient) ClosePosition(ctx context.Context, address, symbol string, request ClosePositionRequest) (WriteResponse, error) {
	market, err := c.marketForSymbol(ctx, symbol)
	if err != nil {
		return writeFailure(err)
	}
	account, err := c.AccountSnapshot(ctx, address)
	if err != nil {
		return writeFailure(err)
	}
	var position *Position
	for i := range account.Positions {
		if strings.EqualFold(account.Positions[i].Symbol, symbol) {
			position = &account.Positions[i]
			break
		}
	}
	if position == nil || normalizeWireDecimal(position.Size) == "0" {
		return writeFailure(errors.New("no open position for symbol"))
	}
	size, err := percentageOfDecimal(position.Size, request.Percent, market.SizePrecision)
	if err != nil {
		return writeFailure(err)
	}
	side := "sell"
	if strings.EqualFold(position.Side, "sell") {
		side = "buy"
	}
	price := request.Price
	tif := "gtc"
	if request.Kind == "market" {
		price, err = aggressivePrice(market.MarkPx, side == "buy", request.SlippagePercent, market.SizePrecision)
		if err != nil {
			return writeFailure(err)
		}
		tif = "ioc"
	}
	order, err := buildLimitOrderWire(market.AssetIndex, side, size, price, tif, true, "")
	if err != nil {
		return writeFailure(err)
	}
	return c.signAndPost(ctx, orderAction{Type: "order", Orders: []wireOrder{order}, Grouping: "na"})
}

func (c *safeClient) marketForSymbol(ctx context.Context, symbol string) (*Market, error) {
	markets, err := c.Markets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range markets {
		if strings.EqualFold(markets[i].Symbol, strings.TrimSpace(symbol)) {
			market := markets[i]
			return &market, nil
		}
	}
	return nil, fmt.Errorf("symbol %q is not a supported perp market", symbol)
}

func (c *safeClient) orderWireForRequest(request OrderRequest, market *Market) (wireOrder, error) {
	isBuy := strings.EqualFold(request.Side, "buy")
	switch request.Kind {
	case "market":
		price, err := aggressivePrice(market.MarkPx, isBuy, request.SlippagePercent, market.SizePrecision)
		if err != nil {
			return wireOrder{}, err
		}
		return buildLimitOrderWire(market.AssetIndex, request.Side, request.Size, price, "ioc", request.ReduceOnly, request.ClientOrderID)
	case "limit":
		return buildLimitOrderWire(market.AssetIndex, request.Side, request.Size, request.Price, request.TimeInForce, request.ReduceOnly, request.ClientOrderID)
	case "stopMarket", "stopLimit", "takeProfitMarket", "takeProfitLimit":
		isMarket := strings.HasSuffix(request.Kind, "Market")
		typeName := "sl"
		if strings.HasPrefix(request.Kind, "takeProfit") {
			typeName = "tp"
		}
		price := firstNonEmptyString(request.TriggerLimitPrice, request.Price)
		if isMarket {
			var err error
			price, err = aggressivePrice(request.TriggerPrice, isBuy, request.SlippagePercent, market.SizePrecision)
			if err != nil {
				return wireOrder{}, err
			}
		}
		return buildTriggerOrderWire(market.AssetIndex, request.Side, request.Size, price, request.TriggerPrice, typeName, isMarket, request.ReduceOnly, request.ClientOrderID)
	default:
		return wireOrder{}, errors.New("unsupported order kind")
	}
}

func (c *safeClient) attachedOrderWire(market *Market, side, size, tpsl string, attached AttachedOrder, slippage string) (wireOrder, error) {
	price := attached.LimitPrice
	isMarket := strings.TrimSpace(price) == ""
	if isMarket {
		var err error
		price, err = aggressivePrice(attached.TriggerPrice, side == "buy", slippage, market.SizePrecision)
		if err != nil {
			return wireOrder{}, err
		}
	}
	return buildTriggerOrderWire(market.AssetIndex, side, size, price, attached.TriggerPrice, tpsl, isMarket, true, "")
}

func buildLimitOrderWire(asset int, side, size, price, tif string, reduceOnly bool, cloid string) (wireOrder, error) {
	if side != "buy" && side != "sell" {
		return wireOrder{}, errors.New("invalid order side")
	}
	wireTIF, err := wireTIF(tif)
	if err != nil {
		return wireOrder{}, err
	}
	cloid, err = normalizeCLOID(cloid)
	if err != nil {
		return wireOrder{}, err
	}
	return wireOrder{
		Asset:         asset,
		IsBuy:         side == "buy",
		LimitPrice:    normalizeWireDecimal(price),
		Size:          normalizeWireDecimal(size),
		ReduceOnly:    reduceOnly,
		OrderType:     wireLimitOrderType{Limit: wireLimit{TIF: wireTIF}},
		ClientOrderID: cloid,
	}, nil
}

func buildTriggerOrderWire(asset int, side, size, price, triggerPrice, tpsl string, isMarket, reduceOnly bool, cloid string) (wireOrder, error) {
	if side != "buy" && side != "sell" {
		return wireOrder{}, errors.New("invalid order side")
	}
	if tpsl != "tp" && tpsl != "sl" {
		return wireOrder{}, errors.New("invalid trigger type")
	}
	cloid, err := normalizeCLOID(cloid)
	if err != nil {
		return wireOrder{}, err
	}
	return wireOrder{
		Asset:      asset,
		IsBuy:      side == "buy",
		LimitPrice: normalizeWireDecimal(price),
		Size:       normalizeWireDecimal(size),
		ReduceOnly: reduceOnly,
		OrderType: wireTriggerOrderType{Trigger: wireTrigger{
			IsMarket:  isMarket,
			TriggerPx: normalizeWireDecimal(triggerPrice),
			TPSL:      tpsl,
		}},
		ClientOrderID: cloid,
	}, nil
}

func wireTIF(tif string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(tif)) {
	case "gtc", "":
		return "Gtc", nil
	case "ioc":
		return "Ioc", nil
	case "alo":
		return "Alo", nil
	default:
		return "", errors.New("invalid time in force")
	}
}

func aggressivePrice(reference string, isBuy bool, slippagePercent string, sizePrecision int) (string, error) {
	referenceValue, err := strconv.ParseFloat(strings.TrimSpace(reference), 64)
	if err != nil || referenceValue <= 0 || math.IsInf(referenceValue, 0) || math.IsNaN(referenceValue) {
		return "", errors.New("market reference price is unavailable")
	}
	slippage := defaultMarketSlippagePercent
	if strings.TrimSpace(slippagePercent) != "" {
		slippage, err = strconv.ParseFloat(strings.TrimSpace(slippagePercent), 64)
		if err != nil || slippage <= 0 || slippage > 5 {
			return "", errors.New("slippage percent must be greater than 0 and at most 5")
		}
	}
	factor := 1 - slippage/100
	if isBuy {
		factor = 1 + slippage/100
	}
	price := referenceValue * factor
	price, err = strconv.ParseFloat(strconv.FormatFloat(price, 'g', 5, 64), 64)
	if err != nil {
		return "", errors.New("failed to round market price")
	}
	decimals := 6 - sizePrecision
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 8 {
		decimals = 8
	}
	power := math.Pow10(decimals)
	price = math.Round(price*power) / power
	return trimFixedDecimal(strconv.FormatFloat(price, 'f', decimals, 64)), nil
}

func normalizeWireDecimal(value string) string {
	value = strings.TrimSpace(value)
	if !strings.Contains(value, ".") {
		return value
	}
	return trimFixedDecimal(value)
}

func trimFixedDecimal(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(value, "0")
		value = strings.TrimRight(value, ".")
	}
	if value == "" || value == "-0" {
		return "0"
	}
	return value
}

func percentageOfDecimal(value string, percent, precision int) (string, error) {
	parsed, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || parsed.Sign() <= 0 {
		return "", errors.New("invalid position size")
	}
	parsed.Mul(parsed, big.NewRat(int64(percent), 100))
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(precision)), nil)
	scaled := new(big.Rat).Mul(parsed, new(big.Rat).SetInt(power))
	units := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if units.Sign() <= 0 {
		return "", errors.New("close size rounds to zero")
	}
	whole := new(big.Int).Quo(new(big.Int).Set(units), power)
	if precision == 0 {
		return whole.String(), nil
	}
	fraction := new(big.Int).Mod(new(big.Int).Set(units), power)
	fractionText := fraction.String()
	if len(fractionText) < precision {
		fractionText = strings.Repeat("0", precision-len(fractionText)) + fractionText
	}
	return trimFixedDecimal(whole.String() + "." + fractionText), nil
}

func normalizeCLOID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	raw := strings.TrimPrefix(value, "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 16 {
		return "", errors.New("client order id must be 16 bytes encoded as 0x plus 32 hex characters")
	}
	return "0x" + raw, nil
}

func parseOIDOrCLOID(value string) (any, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		return normalizeCLOID(value)
	}
	oid, err := strconv.ParseInt(value, 10, 64)
	if err != nil || oid <= 0 {
		return nil, errors.New("order id must be a positive integer or a 16-byte client order id")
	}
	return oid, nil
}

func (c *safeClient) signAndPost(ctx context.Context, action any) (WriteResponse, error) {
	if c.signingKey == nil {
		return writeFailure(errWritesDisabled)
	}
	nonce := c.nextNonce()
	signature, err := signL1ActionForVault(c.signingKey, action, nonce, false, c.vaultAddress)
	if err != nil {
		return writeFailure(fmt.Errorf("sign action: %w", err))
	}
	var vaultAddress any
	if c.vaultAddress != "" {
		vaultAddress = c.vaultAddress
	}
	payload := exchangeEnvelope{Action: action, Nonce: nonce, Signature: signature, VaultAddress: vaultAddress, ExpiresAfter: nil}
	raw, err := c.postExchange(ctx, payload)
	if err != nil {
		return writeFailure(err)
	}
	return parseWriteResponse(raw)
}

func (c *safeClient) nextNonce() int64 {
	c.nonceMu.Lock()
	defer c.nonceMu.Unlock()
	now := time.Now().UnixMilli()
	if now <= c.lastNonce {
		now = c.lastNonce + 1
	}
	c.lastNonce = now
	return now
}

func signL1Action(key *ecdsa.PrivateKey, action any, nonce int64, isMainnet bool) (exchangeSignature, error) {
	return signL1ActionForVault(key, action, nonce, isMainnet, "")
}

func signL1ActionForVault(key *ecdsa.PrivateKey, action any, nonce int64, isMainnet bool, vaultAddress string) (exchangeSignature, error) {
	connectionID, err := actionHashForVault(action, nonce, vaultAddress)
	if err != nil {
		return exchangeSignature{}, err
	}
	source := "b"
	if isMainnet {
		source = "a"
	}
	domainTypeHash := crypto.Keccak256([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	agentTypeHash := crypto.Keccak256([]byte("Agent(string source,bytes32 connectionId)"))
	chainID := make([]byte, 32)
	big.NewInt(1337).FillBytes(chainID)
	domainSeparator := crypto.Keccak256(
		domainTypeHash,
		crypto.Keccak256([]byte("Exchange")),
		crypto.Keccak256([]byte("1")),
		chainID,
		make([]byte, 32),
	)
	messageHash := crypto.Keccak256(
		agentTypeHash,
		crypto.Keccak256([]byte(source)),
		connectionID,
	)
	digest := crypto.Keccak256([]byte{0x19, 0x01}, domainSeparator, messageHash)
	signature, err := crypto.Sign(digest, key)
	if err != nil {
		return exchangeSignature{}, err
	}
	return exchangeSignature{
		R: "0x" + hex.EncodeToString(signature[:32]),
		S: "0x" + hex.EncodeToString(signature[32:64]),
		V: int(signature[64]) + 27,
	}, nil
}

func actionHash(action any, nonce int64) ([]byte, error) {
	return actionHashForVault(action, nonce, "")
}

func actionHashForVault(action any, nonce int64, vaultAddress string) ([]byte, error) {
	if nonce < 0 {
		return nil, errors.New("nonce must not be negative")
	}
	var packed bytes.Buffer
	encoder := msgpack.NewEncoder(&packed)
	encoder.UseCompactInts(true)
	if err := encoder.Encode(action); err != nil {
		return nil, err
	}
	if err := binary.Write(&packed, binary.BigEndian, uint64(nonce)); err != nil {
		return nil, err
	}
	if vaultAddress == "" {
		packed.WriteByte(0)
	} else {
		addressBytes, err := decodeHexAddress(vaultAddress)
		if err != nil {
			return nil, err
		}
		packed.WriteByte(1)
		packed.Write(addressBytes)
	}
	return crypto.Keccak256(packed.Bytes()), nil
}

func isHexAddress(value string) bool {
	_, err := decodeHexAddress(value)
	return err == nil
}

func decodeHexAddress(value string) ([]byte, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 20 {
		return nil, errors.New("address must be 20 bytes encoded as hexadecimal")
	}
	return decoded, nil
}

func (c *safeClient) postExchange(ctx context.Context, payload exchangeEnvelope) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/exchange", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("content-type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxExchangeResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > maxExchangeResponseBytes {
		return nil, errors.New("exchange response exceeded size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("exchange returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, errors.New("exchange returned invalid JSON")
	}
	return raw, nil
}

func parseWriteResponse(raw map[string]any) (WriteResponse, error) {
	result := WriteResponse{Status: "ok"}
	if !strings.EqualFold(asString(raw["status"]), "ok") {
		message := limitedMessage(asString(raw["response"], raw["error"]))
		if message == "" {
			message = "exchange rejected the action"
		}
		result.Status = "error"
		result.Message = message
		return result, errors.New(message)
	}
	response, _ := raw["response"].(map[string]any)
	data, _ := response["data"].(map[string]any)
	statuses, _ := data["statuses"].([]any)
	for _, status := range statuses {
		entry, ok := status.(map[string]any)
		if !ok {
			continue
		}
		if message := limitedMessage(asString(entry["error"])); message != "" {
			result.Status = "error"
			result.Message = message
			return result, errors.New(message)
		}
		if resting, ok := entry["resting"].(map[string]any); ok {
			result.OrderID = asString(resting["oid"])
		}
		if filled, ok := entry["filled"].(map[string]any); ok {
			result.OrderID = asString(filled["oid"])
			result.Filled = asString(filled["totalSz"])
			result.AveragePrice = asString(filled["avgPx"])
		}
	}
	return result, nil
}

func limitedMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

func writeFailure(err error) (WriteResponse, error) {
	return WriteResponse{Status: "error", Message: err.Error()}, err
}
