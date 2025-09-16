package gemini

import (
	"log"

	"github.com/google/generative-ai-go/genai"
)

// 定义 Gemini 的常用模型和角色常量
const (
	ModelGemini15Flash = "gemini-1.5-flash-latest"
	ModelGemini15Pro   = "gemini-1.5-pro-latest"
	DefaultModel       = ModelGemini15Flash

	RoleUser  = "user"
	RoleModel = "model"
)

// ClientOption 是一个用于配置 Client 的函数类型。
type ClientOption func(*Client)

// defaultClient 返回一个带有默认配置的客户端实例。
func defaultClient() *Client {
	return &Client{
		modelName: DefaultModel,
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

// WithEnableContext 启用对话上下文（历史记录）功能。
func WithEnableContext() ClientOption {
	return func(c *Client) {
		c.enableContext = true
	}
}

// ContextMessage 定义了用于初始化的上下文消息结构，对调用者友好。
type ContextMessage struct {
	Role    string // 必须是 "user" 或 "model"
	Content string
}

// WithInitialContextMessages 设置初始的上下文消息。
func WithInitialContextMessages(messages ...*ContextMessage) ClientOption {
	return func(c *Client) {
		if len(messages) == 0 {
			return
		}
		c.enableContext = true
		c.contextHistory = make([]*genai.Content, 0, len(messages))
		for _, msg := range messages {
			// 将友好的 ContextMessage 转换为 genai 库所需的格式
			if msg.Role != RoleUser && msg.Role != RoleModel {
				log.Printf("警告: 无效的 Gemini 角色 '%s'，已跳过该消息。", msg.Role)
				continue
			}
			c.contextHistory = append(c.contextHistory, &genai.Content{
				Parts: []genai.Part{genai.Text(msg.Content)},
				Role:  msg.Role,
			})
		}
	}
}
