package assist

import "context"

type Message struct {
	Role    string
	Content string
}

type Request struct {
	Task           string
	SchemaName     string
	ResponseSchema map[string]any
	Messages       []Message
}

type Response struct {
	RawText string
}

type Client interface {
	Complete(ctx context.Context, request Request) (Response, error)
}
