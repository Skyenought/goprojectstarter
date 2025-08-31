package volc

import (
	"github.com/sashabaranov/go-openai"
)

// 定义火山方舟的常用模型常量
const (
	ModelDoubaoSeedThinking = "doubao-seed-1-6-thinking-250715"
	DefaultModel            = ModelDoubaoSeedThinking
	defaultMaxTokens        = 8192
)

// ClientOption 是一个用于配置 Client 的函数类型。
type ClientOption func(*Client)

// defaultClient 返回一个带有默认配置的客户端实例。
func defaultClient() *Client {
	return &Client{
		modelName:   DefaultModel,
		temperature: 0.7, // 为代码修复设置一个较合理的默认值
		maxTokens:   defaultMaxTokens,
	}
}

// apply 将一组选项应用到客户端。
func (c *Client) apply(opts ...ClientOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// WithModel 设置要使用的模型名称。
func WithModel(name string) ClientOption {
	return func(c *Client) {
		if name != "" {
			c.modelName = name
		}
	}
}

// WithTemperature 设置生成的随机性。
func WithTemperature(temperature float32) ClientOption {
	return func(c *Client) {
		c.temperature = temperature
	}
}

// WithMaxTokens 设置生成的最大 token 数量。
func WithMaxTokens(maxTokens int) ClientOption {
	return func(c *Client) {
		if maxTokens > 0 {
			c.maxTokens = maxTokens
		}
	}
}

// WithEnableContext 启用对话上下文（历史记录）功能。
func WithEnableContext() ClientOption {
	return func(c *Client) {
		c.enableContext = true
	}
}

// ContextMessage 定义了用于初始化的上下文消息结构。
type ContextMessage struct {
	Role    string
	Content string
}

// WithInitialContextMessages 设置初始的上下文消息。
func WithInitialContextMessages(messages ...*ContextMessage) ClientOption {
	return func(c *Client) {
		if len(messages) > 0 {
			c.enableContext = true
			for _, msg := range messages {
				c.contextMessages = append(c.contextMessages, openai.ChatCompletionMessage{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		}
	}
}
