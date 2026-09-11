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
