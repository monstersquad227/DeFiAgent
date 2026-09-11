package main

import (
	"fmt"
	"log"
	"net/http"

	"arbi-mcp-server/config"
	"arbi-mcp-server/controller"
	"arbi-mcp-server/services"
	"arbi-mcp-server/tools"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 初始化配置
	config.Init()
	cfg := config.Get()

	// 初始化以太坊服务 (Mainnet)
	ethSvc, err := services.NewEthereumService(cfg.RPCURL)
	if err != nil {
		log.Fatalf("failed to create mainnet ethereum service: %v", err)
	}

	// 初始化以太坊服务 (Testnet)
	ethSvcTest, err := services.NewEthereumService(cfg.RPCURLTest)
	if err != nil {
		log.Fatalf("failed to create testnet ethereum service: %v", err)
	}

	// 初始化 Accountbalance 服务层
	accSvc := services.NewAccountbalanceService(ethSvc, ethSvcTest)

	// 初始化 DexQuote 服务层
	dexSvc := services.NewDexQuoteService(ethSvc)

	// 初始化 Accountbalance 控制器
	accCtrl := &controller.AccountbalanceController{Svc: accSvc}

	// 初始化 DexQuote 控制器
	dexCtrl := &controller.DexQuoteController{Svc: dexSvc}

	// 创建 MCP Server
	server := mcp.NewServer(
		&mcp.Implementation{Name: cfg.Application.Name, Version: cfg.Application.Version},
		nil,
	)

	// 注册工具
	tools.AccountbalanceTools(server, accCtrl)
	tools.DexQuoteTools(server, dexCtrl)

	// 创建 HTTP Handler
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return server
		},
		&mcp.StreamableHTTPOptions{
			MaxRequestBodyBytes: mcp.DefaultMaxRequestBodyBytes,
		},
	)

	// 启动 Gin HTTP 服务
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	r.POST("/mcp", gin.WrapH(handler))

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Application.Port)
	log.Printf("starting %s v%s on %s", cfg.Application.Name, cfg.Application.Version, addr)
	r.Run(addr)
}
