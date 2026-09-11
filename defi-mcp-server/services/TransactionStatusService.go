package services

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"defi-mcp-server/model"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// ERC-20 Transfer 事件 ABI
const erc20TransferEventABI = `[{"anonymous":false,"inputs":[{"indexed":true,"name":"from","type":"address"},{"indexed":true,"name":"to","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]`

// tokenMeta 代币元数据（复用同一个结构给 mainnet/testnet）
type tokenMeta struct {
	Symbol   string
	Decimals int64
}

// buildAddrToToken 将代币符号→地址 map 反转为 地址→元数据 map
func buildAddrToToken(tokenMap map[string]struct {
	Address  common.Address
	Decimals int64
}) map[common.Address]tokenMeta {
	out := make(map[common.Address]tokenMeta)
	for sym, info := range tokenMap {
		if sym == "WAVAX" {
			continue // 与 AVAX 共用地址，跳过
		}
		out[info.Address] = tokenMeta{Symbol: sym, Decimals: info.Decimals}
	}
	return out
}

type transactionStatusService struct {
	ethMainnet     *EthereumService
	ethTestnet     *EthereumService
	erc20ABI       abi.ABI
	transferTopic  common.Hash
	addrTokenMain  map[common.Address]tokenMeta
	addrTokenTest  map[common.Address]tokenMeta
}

func NewTransactionStatusService(ethMainnet, ethTestnet *EthereumService) TransactionStatusService {
	parsedABI, err := abi.JSON(strings.NewReader(erc20TransferEventABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse ERC-20 Transfer event ABI: %v", err))
	}

	return &transactionStatusService{
		ethMainnet:    ethMainnet,
		ethTestnet:    ethTestnet,
		erc20ABI:      parsedABI,
		transferTopic: parsedABI.Events["Transfer"].ID,
		addrTokenMain: buildAddrToToken(dexTokenMap),
		addrTokenTest: buildAddrToToken(dexTokenMapTestnet),
	}
}

// selectClient 根据 network 选择 client 和 token 反向映射
func (s *transactionStatusService) selectClient(network string) (*EthereumService, map[common.Address]tokenMeta, error) {
	switch network {
	case "mainnet":
		return s.ethMainnet, s.addrTokenMain, nil
	case "testnet":
		return s.ethTestnet, s.addrTokenTest, nil
	default:
		return nil, nil, fmt.Errorf("unsupported network: %s, supported: mainnet, testnet", network)
	}
}

func (s *transactionStatusService) GetTransactionStatus(txHash, network string) (*model.TransactionStatusResponse, error) {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		network = "mainnet"
	}

	client, addrToToken, err := s.selectClient(network)
	if err != nil {
		return nil, err
	}

	hash := common.HexToHash(txHash)

	receipt, err := client.Client.TransactionReceipt(context.Background(), hash)
	tx, _, err2 := client.Client.TransactionByHash(context.Background(), hash)

	// receipt 为 nil → 可能还在 pending
	if receipt == nil && err != nil {
		if err2 != nil {
			return nil, fmt.Errorf("transaction not found: %s", txHash)
		}
		return &model.TransactionStatusResponse{
			TxHash: txHash,
			Status: "Pending",
		}, nil
	}

	resp := &model.TransactionStatusResponse{
		TxHash:  txHash,
		Block:   receipt.BlockNumber.Uint64(),
		GasUsed: receipt.GasUsed,
	}

	// 交易状态
	switch receipt.Status {
	case types.ReceiptStatusSuccessful:
		resp.Status = "Confirmed"
	case types.ReceiptStatusFailed:
		resp.Status = "Failed"
	default:
		resp.Status = "Pending"
	}

	// Gas 费用 (ETH)
	if tx != nil {
		resp.GasETH = s.calcGasETH(tx, receipt)
	}

	// 解析 ERC-20 Transfer 日志 + ETH 转账
	resp.Transfers = s.parseTransfers(receipt.Logs, tx, addrToToken)

	return resp, nil
}

// calcGasETH 计算 gas 费用 (ETH)
func (s *transactionStatusService) calcGasETH(tx *types.Transaction, receipt *types.Receipt) float64 {
	gasUsed := new(big.Int).SetUint64(receipt.GasUsed)
	var gasPrice *big.Int

	if receipt.EffectiveGasPrice != nil && receipt.EffectiveGasPrice.Sign() > 0 {
		gasPrice = receipt.EffectiveGasPrice
	} else if tx.GasPrice() != nil && tx.GasPrice().Sign() > 0 {
		gasPrice = tx.GasPrice()
	} else if tx.GasFeeCap() != nil && tx.GasFeeCap().Sign() > 0 {
		gasPrice = tx.GasFeeCap()
	} else {
		return 0
	}

	costWei := new(big.Int).Mul(gasUsed, gasPrice)
	costETH := new(big.Float).Quo(new(big.Float).SetInt(costWei), big.NewFloat(1e18))
	result, _ := costETH.Float64()
	return result
}

// parseTransfers 从 logs 中解析代币转账
func (s *transactionStatusService) parseTransfers(logs []*types.Log, tx *types.Transaction, addrToToken map[common.Address]tokenMeta) []model.TokenTransfer {
	var transfers []model.TokenTransfer

	for _, l := range logs {
		if len(l.Topics) == 0 || l.Topics[0] != s.transferTopic {
			continue
		}
		tk, ok := addrToToken[l.Address]
		if !ok {
			continue
		}
		if len(l.Topics) < 3 {
			continue
		}

		var ev struct {
			Value *big.Int
		}
		if err := s.erc20ABI.UnpackIntoInterface(&ev, "Transfer", l.Data); err != nil {
			continue
		}
		if ev.Value == nil || ev.Value.Sign() == 0 {
			continue
		}

		from := common.BytesToAddress(l.Topics[1].Bytes())
		to := common.BytesToAddress(l.Topics[2].Bytes())

		amount := weiToFloat(ev.Value, tk.Decimals)
		transfers = append(transfers, model.TokenTransfer{
			Token:  tk.Symbol,
			From:   from.Hex(),
			To:     to.Hex(),
			Amount: amount,
		})
	}

	// ETH 原生转账
	if tx != nil && tx.Value() != nil && tx.Value().Sign() > 0 && tx.To() != nil {
		chainID := tx.ChainId()
		if chainID == nil {
			chainID = big.NewInt(43114) // Avalanche C-Chain
		}
		from, err := types.Sender(types.NewLondonSigner(chainID), tx)
		if err != nil {
			from = common.Address{}
		}
		ethAmount := weiToFloat(tx.Value(), 18)
		transfers = append(transfers, model.TokenTransfer{
			Token:  "ETH",
			From:   from.Hex(),
			To:     tx.To().Hex(),
			Amount: ethAmount,
		})
	}

	return transfers
}