package model

type DexQuoteRequest struct {
	TokenIn  string  `json:"tokenIn" jsonschema:"输入代币符号,如 USDC、ETH"`
	TokenOut string  `json:"tokenOut" jsonschema:"输出代币符号,如 ETH、USDC"`
	Amount   float64 `json:"amount" jsonschema:"输入代币数量(人类可读格式)"`
	Network  string  `json:"network,omitempty" jsonschema:"网络,mainnet 或 testnet,默认 mainnet"`
}

type DexQuoteItem struct {
	Dex       string  `json:"dex"`
	AmountOut float64 `json:"amountOut"`
}

type DexQuoteResponse struct {
	TokenIn  string         `json:"tokenIn"`
	TokenOut string         `json:"tokenOut"`
	Amount   float64        `json:"amount"`
	Quotes   []DexQuoteItem `json:"quotes"`
}