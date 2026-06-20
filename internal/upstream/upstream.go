package upstream

import (
	"context"
	"net/http"
)

// Upstream 定义单个上游的完整职责：token + 请求 → HTTP → 流解析 → 标准化事件。
type Upstream interface {
	Chat(ctx context.Context, token string, req *ChatRequest) (<-chan StreamEvent, error)
	Name() string
}

// Doer 抽象 HTTP 执行。生产注入 curl-impersonate transport；测试注入 fake/stdlib doer。
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}
