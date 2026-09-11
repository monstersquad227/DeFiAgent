package model

type TransactionStatusRequest struct {
	TxHash  string `json:"txHash" jsonschema:"交易哈希"`
	Network string `json:"network,omitempty" jsonschema:"网络,mainnet 或 testnet,默认 mainnet"`
}

type TokenTransfer struct {
	Token  string  `json:"token"`
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
}

type TransactionStatusResponse struct {
	TxHash    string          `json:"txHash"`
	Status    string          `json:"status"`
	Block     uint64          `json:"block"`
	GasETH    float64         `json:"gasETH"`
	GasUsed   uint64          `json:"gasUsed"`
	Transfers []TokenTransfer `json:"transfers"`
}