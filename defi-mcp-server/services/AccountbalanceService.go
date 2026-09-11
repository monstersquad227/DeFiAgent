package services

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// tokenInfo 代币信息
type tokenInfo struct {
	Address  common.Address
	Decimals int64
}

// Avalanche C-Chain Mainnet 代币
var tokenMapMainnet = map[string]tokenInfo{
	"USDC":  {common.HexToAddress("0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E"), 6},
	"USDT":  {common.HexToAddress("0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7"), 6},
	"DAI":   {common.HexToAddress("0xd586E7F844cEa2F87f50152665BCbc2C279D8d70"), 18},
	"WBTC":  {common.HexToAddress("0x50b7545627a5162F82A992c33b87aDc75187B218"), 8},
	"WAVAX": {common.HexToAddress("0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7"), 18},
}

// Avalanche Fuji Testnet 代币
var tokenMapTestnet = map[string]tokenInfo{
	"USDC":  {common.HexToAddress("0x5425890298aed601595a70AB815c96711a31Bc65"), 6},
	"WAVAX": {common.HexToAddress("0xd00ae08403B9bbb9124bB305C09058E32C39A48c"), 18},
}

// erc20ABI ERC-20 balanceOf 的 ABI
const erc20ABI = `[{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"type":"function"}]`

type accountbalanceService struct {
	ethMainnet *EthereumService
	ethTestnet *EthereumService
	erc20ABI   abi.ABI
}

func NewAccountbalanceService(ethMainnet, ethTestnet *EthereumService) AccountbalanceService {
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse ERC-20 ABI: %v", err))
	}
	return &accountbalanceService{ethMainnet: ethMainnet, ethTestnet: ethTestnet, erc20ABI: parsedABI}
}

func (s *accountbalanceService) GetBalance(address, token, network string) ([]map[string]string, error) {
	token = strings.ToUpper(strings.TrimSpace(token))
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		network = "mainnet"
	}

	eth, tokenMap := s.selectNetwork(network)
	if eth == nil {
		return nil, fmt.Errorf("unsupported network: %s, supported: mainnet, testnet", network)
	}

	addr := common.HexToAddress(address)

	if token == "" {
		return s.getAllBalances(eth, tokenMap, addr)
	}
	return s.getSingleBalance(eth, tokenMap, addr, token)
}

func (s *accountbalanceService) selectNetwork(network string) (*EthereumService, map[string]tokenInfo) {
	switch network {
	case "mainnet":
		return s.ethMainnet, tokenMapMainnet
	case "testnet":
		return s.ethTestnet, tokenMapTestnet
	default:
		return nil, nil
	}
}

// getAllBalances 查询所有支持资产的余额
func (s *accountbalanceService) getAllBalances(eth *EthereumService, tokenMap map[string]tokenInfo, addr common.Address) ([]map[string]string, error) {
	var balances []map[string]string

	// AVAX
	balance, err := eth.Client.BalanceAt(context.Background(), addr, nil)
	if err != nil {
		balances = append(balances, map[string]string{"AVAX": fmt.Sprintf("error: %v", err)})
	} else {
		ether := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
		balances = append(balances, map[string]string{"AVAX": ether.Text('f', 6)})
	}

	// ERC-20 tokens
	for _, symbol := range []string{"USDC", "USDT", "DAI", "WBTC", "WAVAX"} {
		tk := tokenMap[symbol]
		result, err := s.queryERC20(eth, addr, tk)
		if err != nil {
			balances = append(balances, map[string]string{symbol: fmt.Sprintf("error: %v", err)})
			continue
		}
		decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(tk.Decimals), nil))
		formatted := new(big.Float).Quo(new(big.Float).SetInt(result), decimals)
		balances = append(balances, map[string]string{symbol: formatted.Text('f', int(tk.Decimals))})
	}

	return balances, nil
}

// getSingleBalance 查询单个资产余额
func (s *accountbalanceService) getSingleBalance(eth *EthereumService, tokenMap map[string]tokenInfo, addr common.Address, token string) ([]map[string]string, error) {
	if token == "AVAX" {
		balance, err := eth.Client.BalanceAt(context.Background(), addr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get AVAX balance: %w", err)
		}
		ether := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
		return []map[string]string{{"AVAX": ether.Text('f', 6)}}, nil
	}

	tk, ok := tokenMap[token]
	if !ok {
		return nil, fmt.Errorf("unsupported token: %s, supported: AVAX, USDC, USDT, DAI, WBTC, WAVAX", token)
	}

	result, err := s.queryERC20(eth, addr, tk)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s balance: %w", token, err)
	}
	decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(tk.Decimals), nil))
	formatted := new(big.Float).Quo(new(big.Float).SetInt(result), decimals)
	return []map[string]string{{token: formatted.Text('f', int(tk.Decimals))}}, nil
}

// queryERC20 调用 ERC-20 balanceOf
func (s *accountbalanceService) queryERC20(eth *EthereumService, owner common.Address, tk tokenInfo) (*big.Int, error) {
	data, err := s.erc20ABI.Pack("balanceOf", owner)
	if err != nil {
		return nil, fmt.Errorf("encode balanceOf: %w", err)
	}
	result, err := eth.Client.CallContract(context.Background(),
		ethereum.CallMsg{To: &tk.Address, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(result), nil
}
