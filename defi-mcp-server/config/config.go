package config

import (
	_ "embed"
	"log"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var configData []byte

// Config 应用配置结构
type Config struct {
	Application ApplicationConfig `yaml:"application"`
	RPCURL      string            `yaml:"rpcurl"`
	RPCURLTest  string            `yaml:"rpcurltest"`
}

// ApplicationConfig 应用基本信息
type ApplicationConfig struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
	Port        int    `yaml:"port"`
}

var (
	globalConfig *Config
	once         sync.Once
)

// Init 初始化全局配置，应在 main 函数中调用
func Init() {
	once.Do(func() {
		var cfg Config
		if err := yaml.Unmarshal(configData, &cfg); err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		globalConfig = &cfg
		log.Printf("config loaded: %s v%s", cfg.Application.Name, cfg.Application.Version)
	})
}

// Get 获取全局配置实例
func Get() *Config {
	if globalConfig == nil {
		log.Fatal("config not initialized, call config.Init() first")
	}
	return globalConfig
}
