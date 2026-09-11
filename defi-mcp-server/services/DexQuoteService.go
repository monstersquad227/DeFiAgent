package services

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"defi-mcp-server/model"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Uniswap V3 Quoter V2 ABI — quoteExactInputSingle
const quoterV2ABI = `[{"inputs":[{"components":[{"internalType":"address","name":"tokenIn","type":"address"},{"internalType":"address","name":"tokenOut","type":"address"},{"internalType":"uint256","name":"amountIn","type":"uint256"},{"internalType":"uint24","name":"fee","type":"uint24"},{"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}],"internalType":"struct IQuoterV2.QuoteExactInputSingleParams","name":"params","type":"tuple"}],"name":"quoteExactInputSingle","outputs":[{"internalType":"uint256","name":"amountOut","type":"uint256"},{"internalType":"uint160","name":"sqrtPriceX96After","type":"uint160"},{"internalType":"uint32","name":"initializedTicksCrossed","type":"uint32"},{"internalType":"uint256","name":"gasEstimate","type":"uint256"}],"stateMutability":"nonpayable","type":"function"}]`

// V2 Router getAmountsOut ABI（Pangolin / Uniswap V2 fork）
const routerV2ABI = `[{"constant":true,"inputs":[{"name":"amountIn","type":"uint256"},{"name":"path","type":"address[]"}],"name":"getAmountsOut","outputs":[{"name":"amounts","type":"uint256[]"}],"type":"function"}]`

// Avalanche C-Chain Mainnet 代币
var dexTokenMap = map[string]struct {
	Address  common.Address
	Decimals int64
}{
	"AVAX":  {common.HexToAddress("0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7"), 18}, // WAVAX
	"WAVAX": {common.HexToAddress("0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7"), 18},
	"USDC":  {common.HexToAddress("0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E"), 6},
	"USDT":  {common.HexToAddress("0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7"), 6},
	"DAI":   {common.HexToAddress("0xd586E7F844cEa2F87f50152665BCbc2C279D8d70"), 18},
	"WBTC":  {common.HexToAddress("0x50b7545627a5162F82A992c33b87aDc75187B218"), 8},
}

// Avalanche Fuji Testnet 代币
var dexTokenMapTestnet = map[string]struct {
	Address  common.Address
	Decimals int64
}{
	"AVAX":  {common.HexToAddress("0xd00ae08403B9bbb9124bB305C09058E32C39A48c"), 18}, // WAVAX
	"WAVAX": {common.HexToAddress("0xd00ae08403B9bbb9124bB305C09058E32C39A48c"), 18},
	"USDC":  {common.HexToAddress("0x5425890298aed601595a70AB815c96711a31Bc65"), 6},
}

// Uniswap V3 Quoter V2 (Avalanche C-Chain)
var uniswapQuoterV2 = common.HexToAddress("0xbe0F5544EC67e9B3b2D979aaA43f18Fd87E6257F")

// Pangolin Router V2 (Uniswap V2 fork)
var pangolinRouter = common.HexToAddress("0xE54Ca86531e17Ef3616d22Ca28b0D458b6C89106")

// Pangolin V3 Quoter — 集中流动性池报价合约（兼容 Uniswap V3 QuoterV2 接口，支持动态手续费）
// 地址来源: https://docs.pangolin.exchange/developers/contracts-and-integration-reference/avalanche-v3
var pangolinV3Quoter = common.HexToAddress("0xA86522CCc412dBC4FA10991900FE46De95983822")

// V3 手续费层级 (1 = 0.01%, 100 = 0.05%, 500 = 0.05%, 3000 = 0.3%, 10000 = 1%)
var feeTiers = []int64{100, 500, 3000, 10000}

type dexQuoteService struct {
	ethMainnet  *EthereumService
	ethTestnet  *EthereumService
	quoterV2ABI abi.ABI
	routerV2ABI abi.ABI
}

func NewDexQuoteService(ethMainnet, ethTestnet *EthereumService) DexQuoteService {
	parsedQuoter, err := abi.JSON(strings.NewReader(quoterV2ABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse quoter V2 ABI: %v", err))
	}
	parsedRouter, err := abi.JSON(strings.NewReader(routerV2ABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse router V2 ABI: %v", err))
	}
	return &dexQuoteService{
		ethMainnet:  ethMainnet,
		ethTestnet:  ethTestnet,
		quoterV2ABI: parsedQuoter,
		routerV2ABI: parsedRouter,
	}
}

func (s *dexQuoteService) GetQuote(tokenIn, tokenOut, network string, amount float64) (*model.DexQuoteResponse, error) {
	tokenIn = strings.ToUpper(strings.TrimSpace(tokenIn))
	tokenOut = strings.ToUpper(strings.TrimSpace(tokenOut))
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "" {
		network = "mainnet"
	}

	if tokenIn == tokenOut {
		return nil, fmt.Errorf("tokenIn and tokenOut must be different")
	}

	client, tokenMap, err := s.selectClient(network)
	if err != nil {
		return nil, err
	}

	tkIn, ok := tokenMap[tokenIn]
	if !ok {
		return nil, fmt.Errorf("unsupported tokenIn: %s, supported: AVAX, WAVAX, USDC, USDT, DAI, WBTC", tokenIn)
	}
	tkOut, ok := tokenMap[tokenOut]
	if !ok {
		return nil, fmt.Errorf("unsupported tokenOut: %s, supported: AVAX, WAVAX, USDC, USDT, DAI, WBTC", tokenOut)
	}

	amountInWei := float64ToWei(amount, tkIn.Decimals)
	path := []common.Address{tkIn.Address, tkOut.Address}

	var quotes []model.DexQuoteItem

	// === Uniswap V3 (Quoter V2) ===
	if bestOut, err := s.quoteUniswapV3(tkIn.Address, tkOut.Address, amountInWei, client); err == nil {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Uniswap",
			AmountOut: weiToFloat(bestOut, tkOut.Decimals),
		})
	} else {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Uniswap",
			AmountOut: 0,
		})
	}

	// === Pangolin 报价（聚合 V2 + 集中流动性池取最优）===
	var bestPangolinOut *big.Int

	// 1) V2 池（getAmountsOut）
	if v2Out, err := s.quotePangolinV2(amountInWei, path, client); err == nil {
		bestPangolinOut = v2Out
	}

	// 2) V3 集中流动性池（Pangolin V3 Quoter，兼容 Uniswap V3 接口）
	if clOut, err := s.quotePangolinV3(tkIn.Address, tkOut.Address, amountInWei, client); err == nil {
		if bestPangolinOut == nil || clOut.Cmp(bestPangolinOut) > 0 {
			bestPangolinOut = clOut
		}
	}

	if bestPangolinOut != nil {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Pangolin",
			AmountOut: weiToFloat(bestPangolinOut, tkOut.Decimals),
		})
	} else {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Pangolin",
			AmountOut: 0,
		})
	}

	return &model.DexQuoteResponse{
		TokenIn:  tokenIn,
		TokenOut: tokenOut,
		Amount:   amount,
		Quotes:   quotes,
	}, nil
}

// selectClient 根据 network 选择 client 和 token map
func (s *dexQuoteService) selectClient(network string) (*EthereumService, map[string]struct {
	Address  common.Address
	Decimals int64
}, error) {
	switch network {
	case "mainnet":
		return s.ethMainnet, dexTokenMap, nil
	case "testnet":
		return s.ethTestnet, dexTokenMapTestnet, nil
	default:
		return nil, nil, fmt.Errorf("unsupported network: %s, supported: mainnet, testnet", network)
	}
}

// quoteUniswapV3 通过 Quoter V2 获取最优报价（多 fee 层级取最优）
func (s *dexQuoteService) quoteUniswapV3(tokenIn, tokenOut common.Address, amountIn *big.Int, eth *EthereumService) (*big.Int, error) {
	var bestOut *big.Int
	var lastErr error
	sqrtPriceLimitX96 := big.NewInt(0) // 无价格限制

	for _, fee := range feeTiers {
		params := struct {
			TokenIn           common.Address
			TokenOut          common.Address
			AmountIn          *big.Int
			Fee               *big.Int
			SqrtPriceLimitX96 *big.Int
		}{
			TokenIn:           tokenIn,
			TokenOut:          tokenOut,
			AmountIn:          amountIn,
			Fee:               big.NewInt(fee),
			SqrtPriceLimitX96: sqrtPriceLimitX96,
		}

		data, err := s.quoterV2ABI.Pack("quoteExactInputSingle", params)
		if err != nil {
			lastErr = err
			continue
		}

		result, err := eth.Client.CallContract(context.Background(),
			ethereum.CallMsg{To: &uniswapQuoterV2, Data: data}, nil)
		if err != nil {
			lastErr = err
			continue
		}

		outputs, err := s.quoterV2ABI.Unpack("quoteExactInputSingle", result)
		if err != nil {
			lastErr = err
			continue
		}
		if len(outputs) == 0 {
			continue
		}

		amountOut, ok := outputs[0].(*big.Int)
		if !ok || amountOut == nil || amountOut.Sign() == 0 {
			continue
		}

		if bestOut == nil || amountOut.Cmp(bestOut) > 0 {
			bestOut = amountOut
		}
	}

	if bestOut == nil {
		if lastErr != nil {
			return nil, fmt.Errorf("Uniswap V3 quote failed: %w", lastErr)
		}
		return nil, fmt.Errorf("Uniswap V3: no valid pool found for this pair")
	}
	return bestOut, nil
}

// quotePangolinV2 通过 Pangolin V2 Router 的 getAmountsOut 获取报价（仅 V2 恒定乘积池）
func (s *dexQuoteService) quotePangolinV2(amountIn *big.Int, path []common.Address, eth *EthereumService) (*big.Int, error) {
	data, err := s.routerV2ABI.Pack("getAmountsOut", amountIn, path)
	if err != nil {
		return nil, fmt.Errorf("encode getAmountsOut: %w", err)
	}

	result, err := eth.Client.CallContract(context.Background(),
		ethereum.CallMsg{To: &pangolinRouter, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("Pangolin Router call failed: %w", err)
	}

	outputs, err := s.routerV2ABI.Unpack("getAmountsOut", result)
	if err != nil {
		return nil, fmt.Errorf("decode getAmountsOut: %w", err)
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("empty result from getAmountsOut")
	}

	amounts, ok := outputs[0].([]*big.Int)
	if !ok || len(amounts) < 2 {
		return nil, fmt.Errorf("unexpected output format")
	}

	return amounts[1], nil
}

// quotePangolinV3 通过 Pangolin V3 Quoter 查询集中流动性池报价（兼容 Uniswap V3 QuoterV2 接口）
func (s *dexQuoteService) quotePangolinV3(tokenIn, tokenOut common.Address, amountIn *big.Int, eth *EthereumService) (*big.Int, error) {
	var bestOut *big.Int
	var lastErr error
	sqrtPriceLimitX96 := big.NewInt(0)

	// Pangolin V3 支持动态手续费，尝试多个 fee tier 取最优
	for _, fee := range feeTiers {
		params := struct {
			TokenIn           common.Address
			TokenOut          common.Address
			AmountIn          *big.Int
			Fee               *big.Int
			SqrtPriceLimitX96 *big.Int
		}{
			TokenIn:           tokenIn,
			TokenOut:          tokenOut,
			AmountIn:          amountIn,
			Fee:               big.NewInt(fee),
			SqrtPriceLimitX96: sqrtPriceLimitX96,
		}

		data, err := s.quoterV2ABI.Pack("quoteExactInputSingle", params)
		if err != nil {
			lastErr = err
			continue
		}

		result, err := eth.Client.CallContract(context.Background(),
			ethereum.CallMsg{To: &pangolinV3Quoter, Data: data}, nil)
		if err != nil {
			lastErr = err
			continue
		}

		outputs, err := s.quoterV2ABI.Unpack("quoteExactInputSingle", result)
		if err != nil {
			lastErr = err
			continue
		}
		if len(outputs) == 0 {
			continue
		}

		amountOut, ok := outputs[0].(*big.Int)
		if !ok || amountOut == nil || amountOut.Sign() == 0 {
			continue
		}

		if bestOut == nil || amountOut.Cmp(bestOut) > 0 {
			bestOut = amountOut
		}
	}

	if bestOut == nil {
		if lastErr != nil {
			return nil, fmt.Errorf("Pangolin V3 quote failed: %w", lastErr)
		}
		return nil, fmt.Errorf("Pangolin V3: no valid pool found for this pair")
	}
	return bestOut, nil
}

// float64ToWei 将人类可读金额转换为最小单位
func float64ToWei(amount float64, decimals int64) *big.Int {
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	amountFloat := new(big.Float).Mul(big.NewFloat(amount), new(big.Float).SetInt(multiplier))
	wei, _ := amountFloat.Int(nil)
	return wei
}

// weiToFloat 将最小单位金额转换为人类可读格式
func weiToFloat(wei *big.Int, decimals int64) float64 {
	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil))
	amountFloat := new(big.Float).Quo(new(big.Float).SetInt(wei), divisor)
	result, _ := amountFloat.Float64()
	return result
}