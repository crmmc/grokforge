package upstream

import (
	"context"

	fhttp "github.com/bogdanfinn/fhttp"
)

// Upstream 定义单个上游的完整职责：token + 请求 → HTTP → 流解析 → 标准化事件。
type Upstream interface {
	Chat(ctx context.Context, token string, req *ChatRequest) (<-chan StreamEvent, error)
	Name() string
}

// Doer 抽象 HTTP 执行。生产注入 tls-client；测试注入 StdlibDoer。
type Doer interface {
	Do(req *fhttp.Request) (*fhttp.Response, error)
}
