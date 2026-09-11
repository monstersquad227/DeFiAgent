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

type TransactionStatusController struct {
	Svc services.TransactionStatusService
}

func (t *TransactionStatusController) GetTransactionStatus(
	ctx context.Context,
	req *mcp.CallToolRequest,
	input model.TransactionStatusRequest,
) (*mcp.CallToolResult, model.TransactionStatusResponse, error) {
	resp, err := t.Svc.GetTransactionStatus(input.TxHash, input.Network)
	if err != nil {
		return nil, model.TransactionStatusResponse{}, err
	}

	// 构建人类可读文本
	var lines []string
	lines = append(lines, fmt.Sprintf("Transaction: %s", resp.TxHash))
	lines = append(lines, fmt.Sprintf("Status: %s", statusIcon(resp.Status)))
	if resp.Status != "Pending" {
		lines = append(lines, fmt.Sprintf("Block: %d", resp.Block))
		lines = append(lines, fmt.Sprintf("Gas: %.6f ETH (Gas Used: %d)", resp.GasETH, resp.GasUsed))
	}

	if len(resp.Transfers) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Token Transfers:")
		for _, tr := range resp.Transfers {
			lines = append(lines, fmt.Sprintf("  %.4f %s (%s → %s)", tr.Amount, tr.Token, shortenAddr(tr.From), shortenAddr(tr.To)))
		}
	}

	text, _ := json.MarshalIndent(resp, "", "  ")

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("%s\n\n%s", strings.Join(lines, "\n"), string(text))},
		},
	}, *resp, nil
}

func statusIcon(status string) string {
	switch status {
	case "Confirmed":
		return "✓ Confirmed"
	case "Failed":
		return "✗ Failed"
	default:
		return "⏳ Pending"
	}
}

func shortenAddr(addr string) string {
	if len(addr) < 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}