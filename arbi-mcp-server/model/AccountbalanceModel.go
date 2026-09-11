package model

type AccountbalanceRequest struct {
	Address string `json:"address" jsonschema:"钱包地址"`
	Token   string `json:"token,omitempty" jsonschema:"代币符号,支持 ETH/USDC/USDT/DAI/WBTC/ARB,不填返回所有"`
	Network string `json:"network,omitempty" jsonschema:"网络,mainnet 或 testnet,默认 mainnet"`
}

type AccountbalanceResponse struct {
	Address  string              `json:"address"`
	Balances []map[string]string `json:"balances"`
}
