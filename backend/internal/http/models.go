package apiv1

import "fake-mex-backend/internal/exchange"

type StreamEnvelope[T any] struct {
	Type       string `json:"type"`
	Symbol     string `json:"symbol,omitempty"`
	Sequence   uint64 `json:"sequence"`
	ServerTime int64  `json:"serverTime"`
	Data       T      `json:"data"`
}

type RequestContext struct {
	RequestID string
}

type TradingStatus struct {
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Network   string `json:"network"`
}

type TradingToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type ProblemResponse = exchange.Problem
