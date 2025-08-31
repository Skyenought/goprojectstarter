package chatgpt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Skyenought/goprojectstarter/pkg/llm"

	"github.com/sashabaranov/go-openai"
)

// Client is a chat GPT client.
type Client struct {
	apiKey    string
	maxTokens int
	ModelName string
	baseurl   string
	Cli       *openai.Client

	/*
		| Temperature Value | Randomness   | Applicable Scenarios                      |
		|----------------|------------------------|-------------------------------------------|
		| 0                 | No randomness         | Factual answers, code generation, technical documentation |
		| 0.5 - 0.7      | Moderate randomness | Conversational systems, content creation, recommendation systems |
		| 1                 | High randomness       | Creative writing, brainstorming, advertising copy |
		| 1.5 - 2         | Extreme randomness  | Artistic creation, game design, exploratory tasks |
	*/
	temperature float32

	roleDesc string // initial role description

	enableContext   bool                           // whether to use assistant context, default is false
	contextMessages []openai.ChatCompletionMessage // initial context messages
}

// NewClient creates a new chat client.
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("API key cannot be empty")
	}

	c := defaultClientOptions()
	c.apply(opts...)

	if c.ModelName == "" {
		c.ModelName = DefaultModel
	}

	if c.roleDesc != "" {
		c.contextMessages = append(c.contextMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: c.roleDesc,
		})
	}

	c.apiKey = apiKey
	c.Cli = openai.NewClient(apiKey)

	return c, nil
}

// Send sends a prompt to the chat gpt and returns the response.
func (c *Client) Send(ctx context.Context, prompt string, files ...string) (string, error) {
	if prompt == "" {
		return "", errors.New("prompt cannot be empty")
	}

	messages, err := c.setMessages(ctx, prompt, files...)
	if err != nil {
		return "", err
	}

	reply, err := c.Cli.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:               c.ModelName,
			Messages:            messages,
			Temperature:         c.temperature,
			MaxCompletionTokens: c.maxTokens,
			MaxTokens:           c.maxTokens, // Deprecated
		},
	)
	if err != nil {
		return "", err
	}

	replyContent := ""
	for _, choice := range reply.Choices {
		replyContent += choice.Message.Content
	}
	if replyContent == "" {
		return "", errors.New("reply content is empty")
	}
	c.appendAssistantContext(prompt, replyContent)

	return replyContent, nil
}

// SendStream sends a prompt to the chat gpt and returns a channel of responses.
func (c *Client) SendStream(ctx context.Context, prompt string, files ...string) *llm.StreamReply {
	response := &llm.StreamReply{Content: make(chan string), Err: error(nil)}

	go func() {
		defer func() { close(response.Content) }()

		messages, err := c.setMessages(ctx, prompt, files...)
		if err != nil {
			response.Err = err
			return
		}

		req := openai.ChatCompletionRequest{
			Model:               c.ModelName,
			Messages:            messages,
			Stream:              true,
			Temperature:         c.temperature,
			MaxCompletionTokens: c.maxTokens,
			MaxTokens:           c.maxTokens, // Deprecated
		}
		stream, err := c.Cli.CreateChatCompletionStream(ctx, req)
		if err != nil {
			response.Err = err
			return
		}
		defer func() {
			_ = stream.Close() //nolint
		}()

		var replyContent string
		defer func() {
			if response.Err == nil && replyContent != "" {
				c.appendAssistantContext(prompt, replyContent)
			}
		}()

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				response.Err = err
				return
			}

			for _, choice := range resp.Choices {
				select {
				case <-ctx.Done():
					response.Err = ctx.Err()
					return
				case response.Content <- choice.Delta.Content:
					replyContent += choice.Delta.Content
				}
			}
		}
	}()

	return response
}

// ListModelNames lists all available model names.
func (c *Client) ListModelNames(ctx context.Context) ([]string, error) {
	list, err := c.Cli.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	var modelNames []string
	for _, model := range list.Models {
		modelNames = append(modelNames, model.ID)
	}

	return modelNames, nil
}

// ListContextMessages list assistant context messages
func (c *Client) ListContextMessages() []*ContextMessage {
	contextMessages := make([]*ContextMessage, 0, len(c.contextMessages))
	for _, message := range c.contextMessages {
		contextMessages = append(contextMessages, &ContextMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return contextMessages
}

// RefreshContext refreshes assistant context
func (c *Client) RefreshContext() {
	if len(c.contextMessages) > 0 {
		c.contextMessages = []openai.ChatCompletionMessage{}
	}
}

// ModifyInitialRole modifies the initial role description.
func (c *Client) ModifyInitialRole(roleDesc string) {
	if roleDesc == "" {
		return
	}
	message := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: roleDesc,
	}

	if len(c.contextMessages) == 0 {
		c.contextMessages = []openai.ChatCompletionMessage{message}
	} else {
		if c.roleDesc == c.contextMessages[0].Content {
			c.contextMessages[0].Content = roleDesc
		} else {
			c.contextMessages = append([]openai.ChatCompletionMessage{message}, c.contextMessages...)
		}
	}
}

func (c *Client) appendAssistantContext(prompt, replyContent string) {
	if c.enableContext && replyContent != "" {
		c.contextMessages = append(c.contextMessages, []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
			{
				Role:    openai.ChatMessageRoleAssistant,
				Content: replyContent,
			},
		}...)
	}
}

func (c *Client) setMessages(ctx context.Context, prompt string, files ...string) ([]openai.ChatCompletionMessage, error) {
	var messages []openai.ChatCompletionMessage

	// history context
	if len(c.contextMessages) > 0 {
		messages = append(messages, c.contextMessages...)
	}

	// file message
	if len(files) > 0 {
		fileIDs, err := c.uploadFiles(ctx, files)
		if err != nil {
			return nil, err
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: fmt.Sprintf("Please refer to the content of the following document ID: %v", fileIDs),
		})
	}

	// user message
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: prompt,
	})

	return messages, nil
}

func (c *Client) uploadFiles(ctx context.Context, files []string) ([]string, error) {
	fileIDs := make([]string, 0, len(files))
	for _, filePath := range files {
		_, name := filepath.Split(filePath)
		fileResp, err := c.Cli.CreateFile(ctx, openai.FileRequest{
			FileName: name,
			FilePath: filePath,
			Purpose:  "assistants", // for assistants
		})
		if err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileResp.ID)
	}
	return fileIDs, nil
}
