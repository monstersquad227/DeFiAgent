package services

import (
	"github.com/ethereum/go-ethereum/ethclient"
)

// EthereumService 封装以太坊 RPC 客户端
type EthereumService struct {
	Client *ethclient.Client
}

// NewEthereumService 创建以太坊服务实例
func NewEthereumService(rpcURL string) (*EthereumService, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	return &EthereumService{Client: client}, nil
}
