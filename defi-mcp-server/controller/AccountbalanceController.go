package controller

import (
	"context"
	"encoding/json"

	"defi-mcp-server/model"
	"defi-mcp-server/services"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AccountbalanceController struct {
	Svc services.AccountbalanceService
}

func (a *AccountbalanceController) GetAccountBalance(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input model.AccountbalanceRequest,
) (*mcp.CallToolResult, model.AccountbalanceResponse, error) {
	balances, err := a.Svc.GetBalance(input.Address, input.Token, input.Network)
	if err != nil {
		return nil, model.AccountbalanceResponse{}, err
	}

	// 格式化文本输出
	text, _ := json.MarshalIndent(balances, "", "  ")

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(text)},
		},
	}, model.AccountbalanceResponse{
		Address:  input.Address,
		Balances: balances,
	}, nil
}
