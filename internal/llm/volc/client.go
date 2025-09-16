package volc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Skyenought/goprojectstarter/internal/llm" // 导入顶层接口包
	"github.com/sashabaranov/go-openai"
)

const (
	arkApiKeyEnvVar = "ARK_API_KEY"
	defaultBaseURL  = "https://ark.cn-beijing.volces.com/api/v3"
)

var _ llm.Assistant = (*Client)(nil)

type Client struct {
	cli *openai.Client

	modelName       string
	temperature     float32
	maxTokens       int
	enableContext   bool
	contextMessages []openai.ChatCompletionMessage
}

// NewClient 创建一个新的火山方舟 LLM 客户端。
func NewClient(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv(arkApiKeyEnvVar)
	if apiKey == "" {
		return nil, fmt.Errorf("环境变量 %s 必须被设置", arkApiKeyEnvVar)
	}

	// 1. 初始化默认配置
	c := defaultClient()
	c.apply(opts...)

	// 3. 创建针对火山方舟的特定配置
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = defaultBaseURL

	// 4. 初始化底层 HTTP 客户端
	c.cli = openai.NewClientWithConfig(config)

	return c, nil
}

func (c *Client) Send(ctx context.Context, prompt string, files ...string) (string, error) {
	if prompt == "" {
		return "", errors.New("prompt cannot be empty")
	}

	messages := c.prepareMessages(prompt)

	req := openai.ChatCompletionRequest{
		Model:               c.modelName,
		Messages:            messages,
		Temperature:         c.temperature,
		MaxCompletionTokens: c.maxTokens,
	}

	resp, err := c.cli.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("LLM 返回了空的 choices 列表")
	}

	replyContent := resp.Choices[0].Message.Content
	c.appendContext(prompt, replyContent) // 辅助函数，添加上下文

	return replyContent, nil
}

func (c *Client) SendStream(ctx context.Context, prompt string, files ...string) *llm.StreamReply {
	reply := &llm.StreamReply{Content: make(chan string)}

	go func() {
		defer close(reply.Content)

		messages := c.prepareMessages(prompt)
		req := openai.ChatCompletionRequest{
			Model:       c.modelName,
			Messages:    messages,
			Temperature: c.temperature,
			MaxTokens:   c.maxTokens,
			Stream:      true,
		}

		stream, err := c.cli.CreateChatCompletionStream(ctx, req)
		if err != nil {
			reply.Err = err
			return
		}
		defer stream.Close()

		var fullContent string
		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				c.appendContext(prompt, fullContent) // 流结束后添加完整上下文
				return
			}
			if err != nil {
				reply.Err = err
				return
			}
			if len(response.Choices) > 0 {
				chunk := response.Choices[0].Delta.Content
				fullContent += chunk
				select {
				case <-ctx.Done():
					reply.Err = ctx.Err()
					return
				case reply.Content <- chunk:
				}
			}
		}
	}()

	return reply
}

// RefreshContext 实现 Assistant 接口的 RefreshContext 方法。
func (c *Client) RefreshContext() {
	c.contextMessages = nil
}

// ListModelNames 实现 Assistant 接口的 ListModelNames 方法。
func (c *Client) ListModelNames(ctx context.Context) ([]string, error) {
	models, err := c.cli.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, model := range models.Models {
		names = append(names, model.ID)
	}
	return names, nil
}

// prepareMessages 是一个内部辅助函数，用于构建发送给 API 的消息列表。
func (c *Client) prepareMessages(prompt string) []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, 0, len(c.contextMessages)+1)
	if c.enableContext {
		messages = append(messages, c.contextMessages...)
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})
	return messages
}

func (c *Client) appendContext(prompt, reply string) {
	if c.enableContext {
		c.contextMessages = append(c.contextMessages,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: reply},
		)
	}
}
