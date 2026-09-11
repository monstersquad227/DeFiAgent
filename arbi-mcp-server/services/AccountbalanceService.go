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

// Arbitrum Mainnet 代币
var tokenMapMainnet = map[string]tokenInfo{
	"USDC": {common.HexToAddress("0xaf88d065e77c8cC2239327C5EDb3A432268e5831"), 6},
	"USDT": {common.HexToAddress("0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9"), 6},
	"DAI":  {common.HexToAddress("0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1"), 18},
	"WBTC": {common.HexToAddress("0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f"), 8},
	"ARB":  {common.HexToAddress("0x912CE59144191C1204E64559FE8253a0e49E6548"), 18},
}

// Arbitrum Sepolia Testnet 代币
var tokenMapTestnet = map[string]tokenInfo{
	"USDC": {common.HexToAddress("0x75faf114eafb1BDbe2Fc6eedaBfD18A7d4d0F06e"), 6},
	"USDT": {common.HexToAddress("0x3e2E9E4E6d4B0e6C0eF4E0B0e0B0e0b0E0b0E0b0"), 6},
	"DAI":  {common.HexToAddress("0x4D372cF0E7B2e5E3C5E3C5E3C5E3C5E3C5E3C5E3"), 18},
	"WBTC": {common.HexToAddress("0x8f3Cf7ad23Cd3CaF9737af9c0C0E0E0E0E0E0E0E"), 8},
	"ARB":  {common.HexToAddress("0x1a4d3B4f2C0B0e0B0e0B0e0B0e0B0e0B0e0B0e0B0"), 18},
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

	// ETH
	balance, err := eth.Client.BalanceAt(context.Background(), addr, nil)
	if err != nil {
		balances = append(balances, map[string]string{"ETH": fmt.Sprintf("error: %v", err)})
	} else {
		ether := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
		balances = append(balances, map[string]string{"ETH": ether.Text('f', 6)})
	}

	// ERC-20 tokens
	for _, symbol := range []string{"USDC", "USDT", "DAI", "WBTC", "ARB"} {
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
	if token == "ETH" {
		balance, err := eth.Client.BalanceAt(context.Background(), addr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get ETH balance: %w", err)
		}
		ether := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
		return []map[string]string{{"ETH": ether.Text('f', 6)}}, nil
	}

	tk, ok := tokenMap[token]
	if !ok {
		return nil, fmt.Errorf("unsupported token: %s, supported: ETH, USDC, USDT, DAI, WBTC, ARB", token)
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
