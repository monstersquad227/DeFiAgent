package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"defi-mcp-server/model"
	"defi-mcp-server/services"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DexQuoteController struct {
	Svc services.DexQuoteService
}

func (d *DexQuoteController) GetDexQuote(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input model.DexQuoteRequest,
) (*mcp.CallToolResult, model.DexQuoteResponse, error) {
	resp, err := d.Svc.GetQuote(input.TokenIn, input.TokenOut, input.Network, input.Amount)
	if err != nil {
		return nil, model.DexQuoteResponse{}, err
	}

	// 构建人类可读的文本输出
	var textLines []string
	textLines = append(textLines, fmt.Sprintf("%s → %s 报价:", resp.TokenIn, resp.TokenOut))
	for _, q := range resp.Quotes {
		textLines = append(textLines, fmt.Sprintf("%s: %.0f %s → %.4f %s",
			q.Dex, resp.Amount, resp.TokenIn, q.AmountOut, resp.TokenOut))
	}

	text, _ := json.MarshalIndent(resp.Quotes, "", "  ")

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("%s\n\n%s", strings.Join(textLines, "\n"), string(text))},
		},
	}, *resp, nil
}