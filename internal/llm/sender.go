package llm

import "context"

type Assistant interface {
	// Send 发送一个请求，并一次性返回完整的响应。
	Send(ctx context.Context, prompt string, files ...string) (string, error)

	// SendStream 以流式方式发送请求，实时返回内容片段。
	SendStream(ctx context.Context, prompt string, files ...string) *StreamReply

	// RefreshContext 清空当前客户端维护的对话上下文（历史记录）。
	RefreshContext()

	// ListModelNames 获取该平台支持的模型名称列表。
	ListModelNames(ctx context.Context) ([]string, error)
}

type StreamReply struct {
	Content chan string
	Err     error
}
