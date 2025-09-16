package common

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Skyenought/goprojectstarter/internal/llm"
	"github.com/Skyenought/goprojectstarter/internal/llm/deepseek"
	"github.com/Skyenought/goprojectstarter/internal/llm/gemini"
	"github.com/Skyenought/goprojectstarter/internal/llm/volc"
	"gopkg.in/yaml.v3"
)

type LLMConfig struct {
	Default   string `yaml:"default"`
	Providers map[string]struct {
		Models []string `yaml:"models"`
	} `yaml:"providers"`
}

// GenWithDefaultLLM 是一个高级辅助函数，它负责：
// 1. 读取 `.goprojectstarter.yaml` 配置文件。
// 2. 根据配置中的 `default` 字段确定要使用的 LLM 提供商和模型。
// 3. 从环境变量中获取对应的 API Key。
// 4. 初始化选择的 LLM 客户端。
// 5. 发送 prompt 并返回结果。
func GenWithDefaultLLM(prompt string) (string, error) {
	// 加载 LLM 配置
	config, err := loadLLMConfig()
	if err != nil {
		return "", fmt.Errorf("无法加载 LLM 配置: %w", err)
	}

	// 解析默认的提供商和模型
	parts := strings.Split(config.Default, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("配置文件中 'default' LLM 格式无效 (应为 'provider:model'): %s", config.Default)
	}
	provider, model := parts[0], parts[1]

	var client llm.Assistant // 使用顶层 Assistant 接口
	var apiKey string

	fmt.Printf("   - 使用默认 LLM: %s (%s)\n", provider, model)

	// 根据提供商选择并初始化客户端
	switch provider {
	case "gemini":
		// Gemini 客户端通过环境变量自动读取 API key
		client, err = gemini.NewClient(gemini.WithModel(model))
	case "deepseek":
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			return "", fmt.Errorf("环境变量 DEEPSEEK_API_KEY 未设置")
		}
		client, err = deepseek.NewClient(apiKey, deepseek.WithModel(model))
	case "volc":
		apiKey = os.Getenv("ARK_API_KEY")
		if apiKey == "" {
			return "", fmt.Errorf("环境变量 ARK_API_KEY 未设置")
		}
		client, err = volc.NewClient(volc.WithModel(model))
	default:
		return "", fmt.Errorf("不支持的 LLM 提供商: %s", provider)
	}

	if err != nil {
		return "", fmt.Errorf("为 %s 创建 LLM 客户端失败: %w", provider, err)
	}

	// 为 API 调用设置一个超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 使用通用的 Send 方法发送请求
	return client.Send(ctx, prompt)
}

// loadLLMConfig 读取并解析 .goprojectstarter.yaml 文件
func loadLLMConfig() (*LLMConfig, error) {
	file, err := os.ReadFile(".goprojectstarter.yaml")
	if err != nil {
		return nil, err
	}
	// 我们只关心文件中的 'llm' 顶级键
	var config struct {
		LLM LLMConfig `yaml:"llm"`
	}
	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}
	return &config.LLM, nil
}
