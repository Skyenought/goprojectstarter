package gemini

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Skyenought/goprojectstarter/internal/llm"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	geminiApiKeyEnvVar = "GEMINI_API_KEY"
)

// 确保 Client 结构体实现了 llm.Assistant 接口。
var _ llm.Assistant = (*Client)(nil)

// Client 是与 Gemini 大模型服务交互的客户端。
type Client struct {
	cli   *genai.Client          // 底层使用官方 genai 客户端
	model *genai.GenerativeModel // 预先初始化的模型实例

	// 配置项
	modelName      string
	enableContext  bool
	contextHistory []*genai.Content // 存储对话历史，使用 genai 的格式
}

// NewClient 创建一个新的 Gemini LLM 客户端。
func NewClient(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv(geminiApiKeyEnvVar)
	if apiKey == "" {
		return nil, fmt.Errorf("环境变量 %s 必须被设置", geminiApiKeyEnvVar)
	}

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("创建 genai 客户端失败: %w", err)
	}

	c := defaultClient()
	c.apply(opts...)

	c.cli = genaiClient
	c.model = genaiClient.GenerativeModel(c.modelName)

	return c, nil
}

// Send 实现 Assistant 接口的 Send 方法。
func (c *Client) Send(ctx context.Context, prompt string, files ...string) (string, error) {
	if prompt == "" {
		return "", errors.New("prompt cannot be empty")
	}

	// 使用 ChatSession 可以方便地管理上下文
	session := c.model.StartChat()
	if c.enableContext && len(c.contextHistory) > 0 {
		session.History = c.contextHistory
	}

	resp, err := session.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	// 从响应中提取文本内容
	textContent := extractTextFromResponse(resp)
	if textContent == "" {
		return "", errors.New("LLM 返回了空的内容")
	}

	// 如果启用了上下文，则更新历史记录
	if c.enableContext {
		c.contextHistory = session.History
	}

	return textContent, nil
}

// SendStream 实现 Assistant 接口的 SendStream 方法。
func (c *Client) SendStream(ctx context.Context, prompt string, files ...string) *llm.StreamReply {
	reply := &llm.StreamReply{Content: make(chan string)}

	go func() {
		defer close(reply.Content)

		session := c.model.StartChat()
		if c.enableContext && len(c.contextHistory) > 0 {
			session.History = c.contextHistory
		}

		iter := session.SendMessageStream(ctx, genai.Text(prompt))
		var fullContent strings.Builder

		for {
			resp, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				break // 流结束
			}
			if err != nil {
				reply.Err = err
				return
			}

			chunk := extractTextFromResponse(resp)
			fullContent.WriteString(chunk)

			select {
			case <-ctx.Done():
				reply.Err = ctx.Err()
				return
			case reply.Content <- chunk:
			}
		}

		// 流结束后，如果启用了上下文，则更新历史记录
		if c.enableContext {
			c.contextHistory = session.History
		}
	}()

	return reply
}

// RefreshContext 实现 Assistant 接口的 RefreshContext 方法。
func (c *Client) RefreshContext() {
	c.contextHistory = nil
}

// ListModelNames 实现 Assistant 接口的 ListModelNames 方法。
func (c *Client) ListModelNames(ctx context.Context) ([]string, error) {
	iter := c.cli.ListModels(ctx)
	var names []string
	for {
		model, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		names = append(names, model.Name)
	}
	return names, nil
}

// extractTextFromResponse 是一个辅助函数，用于从 Gemini 的响应中健壮地提取文本。
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
	var builder strings.Builder
	if resp == nil {
		return ""
	}
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					builder.WriteString(string(txt))
				}
			}
		}
	}
	return builder.String()
}
