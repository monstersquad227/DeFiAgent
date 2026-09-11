package services

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"arbi-mcp-server/model"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Uniswap V3 Quoter V2 ABI — quoteExactInputSingle
const quoterV2ABI = `[{"inputs":[{"components":[{"internalType":"address","name":"tokenIn","type":"address"},{"internalType":"address","name":"tokenOut","type":"address"},{"internalType":"uint256","name":"amountIn","type":"uint256"},{"internalType":"uint24","name":"fee","type":"uint24"},{"internalType":"uint160","name":"sqrtPriceLimitX96","type":"uint160"}],"internalType":"struct IQuoterV2.QuoteExactInputSingleParams","name":"params","type":"tuple"}],"name":"quoteExactInputSingle","outputs":[{"internalType":"uint256","name":"amountOut","type":"uint256"},{"internalType":"uint160","name":"sqrtPriceX96After","type":"uint160"},{"internalType":"uint32","name":"initializedTicksCrossed","type":"uint32"},{"internalType":"uint256","name":"gasEstimate","type":"uint256"}],"stateMutability":"nonpayable","type":"function"}]`

// V2 Router getAmountsOut ABI（Camelot V3 仍兼容）
const routerV2ABI = `[{"constant":true,"inputs":[{"name":"amountIn","type":"uint256"},{"name":"path","type":"address[]"}],"name":"getAmountsOut","outputs":[{"name":"amounts","type":"uint256[]"}],"type":"function"}]`

// Camelot (Algebra) Quoter ABI — quoteExactInputSingle
const camelotQuoterABI = `[{"inputs":[{"internalType":"address","name":"tokenIn","type":"address"},{"internalType":"address","name":"tokenOut","type":"address"},{"internalType":"uint256","name":"amountIn","type":"uint256"},{"internalType":"uint160","name":"limitSqrtPrice","type":"uint160"}],"name":"quoteExactInputSingle","outputs":[{"internalType":"uint256","name":"amountOut","type":"uint256"},{"internalType":"uint16","name":"fee","type":"uint16"}],"stateMutability":"nonpayable","type":"function"}]`

// Arbitrum Mainnet 代币
var dexTokenMap = map[string]struct {
	Address  common.Address
	Decimals int64
}{
	"ETH":  {common.HexToAddress("0x82aF49447D8a07e3bd95BD0d56f35241523fBab1"), 18}, // WETH
	"WETH": {common.HexToAddress("0x82aF49447D8a07e3bd95BD0d56f35241523fBab1"), 18},
	"USDC": {common.HexToAddress("0xaf88d065e77c8cC2239327C5EDb3A432268e5831"), 6},
	"USDT": {common.HexToAddress("0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9"), 6},
	"DAI":  {common.HexToAddress("0xDA10009cBd5D07dd0CeCc66161FC93D7c9000da1"), 18},
	"WBTC": {common.HexToAddress("0x2f2a2543B76A4166549F7aaB2e75Bef0aefC5B0f"), 8},
	"ARB":  {common.HexToAddress("0x912CE59144191C1204E64559FE8253a0e49E6548"), 18},
}

// Uniswap V3 Quoter V2 (跨链确定性地址)
var uniswapQuoterV2 = common.HexToAddress("0x61fFE014bA17989E743c5F6cB21bF9697530B21e")

// Camelot Router v3
var camelotRouterV3 = common.HexToAddress("0x1F721E2E82F667F6CE4eA07A5958cF098D339e18")

// Camelot Quoter (Algebra AMMv3)
var camelotQuoter = common.HexToAddress("0x0Fc73040b26E9bC8514fA028D998E73A254Fa76E")

// V3 手续费层级 (1 = 0.01%, 100 = 0.05%, 500 = 0.05%, 3000 = 0.3%, 10000 = 1%)
var feeTiers = []int64{100, 500, 3000, 10000}

type dexQuoteService struct {
	ethMainnet      *EthereumService
	quoterV2ABI     abi.ABI
	routerV2ABI     abi.ABI
	camelotQuoterABI abi.ABI
}

func NewDexQuoteService(ethMainnet *EthereumService) DexQuoteService {
	parsedQuoter, err := abi.JSON(strings.NewReader(quoterV2ABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse quoter V2 ABI: %v", err))
	}
	parsedRouter, err := abi.JSON(strings.NewReader(routerV2ABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse router V2 ABI: %v", err))
	}
	parsedCamelotQuoter, err := abi.JSON(strings.NewReader(camelotQuoterABI))
	if err != nil {
		panic(fmt.Sprintf("failed to parse Camelot Quoter ABI: %v", err))
	}
	return &dexQuoteService{
		ethMainnet:       ethMainnet,
		quoterV2ABI:      parsedQuoter,
		routerV2ABI:      parsedRouter,
		camelotQuoterABI: parsedCamelotQuoter,
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
	if network != "mainnet" {
		return nil, fmt.Errorf("unsupported network: %s, only mainnet is supported for DEX quotes", network)
	}

	tkIn, ok := dexTokenMap[tokenIn]
	if !ok {
		return nil, fmt.Errorf("unsupported tokenIn: %s, supported: ETH, WETH, USDC, USDT, DAI, WBTC, ARB", tokenIn)
	}
	tkOut, ok := dexTokenMap[tokenOut]
	if !ok {
		return nil, fmt.Errorf("unsupported tokenOut: %s, supported: ETH, WETH, USDC, USDT, DAI, WBTC, ARB", tokenOut)
	}

	amountInWei := float64ToWei(amount, tkIn.Decimals)
	path := []common.Address{tkIn.Address, tkOut.Address}

	var quotes []model.DexQuoteItem

	// === Uniswap V3 (Quoter V2) ===
	if bestOut, err := s.quoteUniswapV3(tkIn.Address, tkOut.Address, amountInWei); err == nil {
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

	// === Camelot (先试 Router V3 getAmountsOut，失败则用 Quoter) ===
	if amountOutWei, err := s.quoteCamelotV3(amountInWei, path); err == nil {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Camelot",
			AmountOut: weiToFloat(amountOutWei, tkOut.Decimals),
		})
	} else if amountOutWei, qErr := s.quoteCamelotQuoter(tkIn.Address, tkOut.Address, amountInWei); qErr == nil {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Camelot",
			AmountOut: weiToFloat(amountOutWei, tkOut.Decimals),
		})
	} else {
		quotes = append(quotes, model.DexQuoteItem{
			Dex:       "Camelot",
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

// quoteUniswapV3 通过 Quoter V2 获取最优报价（多 fee 层级取最优）
func (s *dexQuoteService) quoteUniswapV3(tokenIn, tokenOut common.Address, amountIn *big.Int) (*big.Int, error) {
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

		result, err := s.ethMainnet.Client.CallContract(context.Background(),
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

// quoteCamelotV3 通过 Camelot Router v3 的 getAmountsOut 获取报价
func (s *dexQuoteService) quoteCamelotV3(amountIn *big.Int, path []common.Address) (*big.Int, error) {
	data, err := s.routerV2ABI.Pack("getAmountsOut", amountIn, path)
	if err != nil {
		return nil, fmt.Errorf("encode getAmountsOut: %w", err)
	}

	result, err := s.ethMainnet.Client.CallContract(context.Background(),
		ethereum.CallMsg{To: &camelotRouterV3, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("Camelot Router call failed: %w", err)
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

// quoteCamelotQuoter 通过 Camelot Quoter (Algebra) 获取报价（回退方案）
func (s *dexQuoteService) quoteCamelotQuoter(tokenIn, tokenOut common.Address, amountIn *big.Int) (*big.Int, error) {
	limitSqrtPrice := big.NewInt(0) // 无价格限制

	data, err := s.camelotQuoterABI.Pack("quoteExactInputSingle", tokenIn, tokenOut, amountIn, limitSqrtPrice)
	if err != nil {
		return nil, fmt.Errorf("encode Camelot quoteExactInputSingle: %w", err)
	}

	result, err := s.ethMainnet.Client.CallContract(context.Background(),
		ethereum.CallMsg{To: &camelotQuoter, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("Camelot Quoter call failed: %w", err)
	}

	outputs, err := s.camelotQuoterABI.Unpack("quoteExactInputSingle", result)
	if err != nil {
		return nil, fmt.Errorf("decode Camelot quoteExactInputSingle: %w", err)
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("empty result from Camelot Quoter")
	}

	amountOut, ok := outputs[0].(*big.Int)
	if !ok || amountOut == nil || amountOut.Sign() == 0 {
		return nil, fmt.Errorf("zero amountOut from Camelot Quoter")
	}

	return amountOut, nil
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