package services

import "defi-mcp-server/model"

type AccountbalanceService interface {
	GetBalance(address, token, network string) ([]map[string]string, error)
}

type DexQuoteService interface {
	GetQuote(tokenIn, tokenOut, network string, amount float64) (*model.DexQuoteResponse, error)
}

type TransactionStatusService interface {
	GetTransactionStatus(txHash, network string) (*model.TransactionStatusResponse, error)
}
