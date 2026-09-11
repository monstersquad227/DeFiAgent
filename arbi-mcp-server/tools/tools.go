package tools

import (
	"arbi-mcp-server/controller"
	"arbi-mcp-server/model"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func AccountbalanceTools(server *mcp.Server, ctrl *controller.AccountbalanceController) {
	inputSchema, err := jsonschema.For[model.AccountbalanceRequest](&jsonschema.ForOptions{})
	if err != nil {
		panic(err)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_balance",
		Description: "获取 Arbitrum 链上资产余额。支持 Mainnet/Testnet,确认 ETH/USDC/USDT/DAI/WBTC/ARB。",
		InputSchema: inputSchema,
	}, ctrl.GetAccountBalance)
}

func DexQuoteTools(server *mcp.Server, ctrl *controller.DexQuoteController) {
	inputSchema, err := jsonschema.For[model.DexQuoteRequest](&jsonschema.ForOptions{})
	if err != nil {
		panic(err)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_dex_quote",
		Description: "获取 Arbitrum 链上 DEX 报价，对比 Uniswap 和 Camelot 的最优价格。支持 ETH/WETH/USDC/USDT/DAI/WBTC/ARB。",
		InputSchema: inputSchema,
	}, ctrl.GetDexQuote)
}

func TransactionStatusTools(server *mcp.Server, ctrl *controller.TransactionStatusController) {
	inputSchema, err := jsonschema.For[model.TransactionStatusRequest](&jsonschema.ForOptions{})
	if err != nil {
		panic(err)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_transaction_status",
		Description: "查询 Arbitrum 链上交易状态，包括确认状态、区块号、Gas 费用和代币转账详情。支持 ETH/USDC/USDT/DAI/WBTC/ARB 转账识别。",
		InputSchema: inputSchema,
	}, ctrl.GetTransactionStatus)
}
