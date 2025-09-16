package chatgpt

import "github.com/sashabaranov/go-openai"

const (
	ModelGPT3Dot5Turbo = openai.GPT3Dot5Turbo
	ModelGPT4          = openai.GPT4
	ModelGPT4Turbo     = openai.GPT4Turbo
	ModelGPT4o         = openai.GPT4o // default
	ModelGPT4oMini     = openai.GPT4oMini
	ModelO1Mini        = openai.O1Mini
	ModelO1Preview     = openai.O1Preview

	DefaultModel     = ModelGPT4o
	defaultMaxTokens = 8192
)

// ClientOption is a function that sets a Client option.
type ClientOption func(*Client)

func defaultClientOptions() *Client {
	return &Client{
		enableContext: false, // default is false
		maxTokens:     defaultMaxTokens,
		temperature:   0.0,
	}
}

func (c *Client) apply(opts ...ClientOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// WithMaxTokens sets the maximum number of tokens
func WithMaxTokens(maxTokens int) ClientOption {
	return func(c *Client) {
		if maxTokens < 1000 {
			c.maxTokens = defaultMaxTokens
		}
		c.maxTokens = maxTokens
	}
}

// WithModel sets the model name
func WithModel(name string) ClientOption {
	return func(c *Client) {
		c.ModelName = name
	}
}

// WithTemperature sets the temperature
func WithTemperature(temperature float32) ClientOption {
	return func(c *Client) {
		c.temperature = temperature
	}
}

// WithInitialRole sets the initial role type
func WithInitialRole(roleDesc string) ClientOption {
	return func(c *Client) {
		c.roleDesc = roleDesc
	}
}

// WithEnableContext sets assistant context
func WithEnableContext() ClientOption {
	return func(c *Client) {
		c.enableContext = true
	}
}

func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		if url != "" {
			c.baseurl = url
		}
	}
}

// ContextMessage chat history message
type ContextMessage struct {
	Role    string `json:"role"` // system, user, assistant, etc.
	Content string `json:"content"`
}

// WithInitialContextMessages sets initial context messages, automatically set enableContext to true
func WithInitialContextMessages(messages ...*ContextMessage) ClientOption {
	return func(c *Client) {
		if len(messages) > 0 {
			c.enableContext = true
			for _, message := range messages {
				c.contextMessages = append(c.contextMessages, openai.ChatCompletionMessage{
					Role:    message.Role,
					Content: message.Content,
				})
			}
		}
	}
}
