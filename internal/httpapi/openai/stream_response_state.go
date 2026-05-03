package openai

import (
	"net/http"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
)

type streamResponseState struct {
	h         *Handler
	r         *http.Request
	writer    *httpapi.SSEWriter
	flusher   http.Flusher
	cfg       *config.Config
	adapter   *chatStreamAdapter
	mediaGate *streamMediaGate
	rewriter  *mediaRewriter
}

type streamResponseOptions struct {
	h       *Handler
	r       *http.Request
	writer  *httpapi.SSEWriter
	flusher http.Flusher
	req     *ChatRequest
	cfg     *config.Config
}

func newStreamResponseState(opts streamResponseOptions) *streamResponseState {
	return &streamResponseState{
		h:         opts.h,
		r:         opts.r,
		writer:    opts.writer,
		flusher:   opts.flusher,
		cfg:       opts.cfg,
		adapter:   newChatStreamAdapter(opts.req, opts.cfg),
		mediaGate: &streamMediaGate{},
	}
}

func (s *streamResponseState) handleEvent(event flow.StreamEvent) error {
	s.ensureMediaRewriter(event)
	return s.writeChunks(s.adapter.HandleEvent(event))
}

func (s *streamResponseState) ensureMediaRewriter(event flow.StreamEvent) {
	if s.rewriter != nil || event.Downloader == nil || s.cfg == nil || !s.cfg.App.MediaGenerationEnabled {
		return
	}
	s.rewriter = newMediaRewriter(event.Downloader)
}

func (s *streamResponseState) finish() error {
	if err := s.flushMediaGate(); err != nil {
		return err
	}
	return s.writeChunks(s.adapter.FinishChunks())
}

func (s *streamResponseState) flushMediaGate() error {
	tail, err := s.mediaGate.flush(s.r.Context(), s.rewriter)
	if err != nil {
		return err
	}
	if tail == "" {
		return nil
	}
	if err := s.writer.WriteSSE(s.adapter.chunk(chatStreamDelta{Content: tail}, nil)); err != nil {
		return err
	}
	return nil
}

func (s *streamResponseState) writeChunks(chunks []chatStreamChunk) error {
	for _, chunk := range chunks {
		guarded, emit, err := s.guardChunk(chunk)
		if err != nil {
			return err
		}
		if !emit {
			continue
		}
		if err := s.writer.WriteSSE(guarded); err != nil {
			return err
		}
	}
	return nil
}

func (s *streamResponseState) writeError(err error) {
	_, apiErr := httpapi.MapXAIError(err)
	s.writer.WriteSSEError(apiErr)
}

func (s *streamResponseState) guardChunk(chunk chatStreamChunk) (chatStreamChunk, bool, error) {
	if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
		return chunk, true, nil
	}
	text, err := s.mediaGate.push(s.r.Context(), s.rewriter, chunk.Choices[0].Delta.Content)
	if err != nil {
		return chatStreamChunk{}, false, err
	}
	if text == "" && isContentOnlyChunk(chunk) {
		return chatStreamChunk{}, false, nil
	}
	chunk.Choices[0].Delta.Content = text
	return chunk, true, nil
}

func isContentOnlyChunk(chunk chatStreamChunk) bool {
	if len(chunk.Choices) != 1 || chunk.Choices[0].FinishReason != nil {
		return false
	}
	delta := chunk.Choices[0].Delta
	return delta.Role == "" && delta.Content != "" && len(delta.ToolCalls) == 0
}
