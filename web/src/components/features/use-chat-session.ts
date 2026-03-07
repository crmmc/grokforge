import * as React from 'react'
import { sendChatMessage, getApiKey, type ChatMessage } from '@/lib/function-api'
import { parseThinkContent } from '@/lib/think-parser'
import { useTranslation } from '@/lib/i18n/context'

interface UseChatSessionOptions {
  selectedModel: string
  updateMessages: (id: string, msgs: ChatMessage[]) => void
}

export function useChatSession({ selectedModel, updateMessages }: UseChatSessionOptions) {
  const { t } = useTranslation()
  const [isStreaming, setIsStreaming] = React.useState(false)
  const [streamingContent, setStreamingContent] = React.useState('')
  const [error, setError] = React.useState<string | null>(null)
  const [streamStats, setStreamStats] = React.useState<{ ttft: number; total: number } | null>(null)
  const abortRef = React.useRef<AbortController | null>(null)
  const streamTimerRef = React.useRef<{ startedAt: number; firstTokenAt: number | null }>({ startedAt: 0, firstTokenAt: null })
  const streamingContentRef = React.useRef('')

  const buildAssistantReply = React.useCallback((msgs: ChatMessage[], content: string) => {
    const answer = parseThinkContent(content).answer.trim()
    if (!answer) return null
    return [...msgs, { id: crypto.randomUUID(), role: 'assistant' as const, content: answer }]
  }, [])

  const doSend = async (msgs: ChatMessage[], convId: string) => {
    if (!getApiKey()) { setError(t.function.configureApiKey); return }

    updateMessages(convId, msgs)
    setError(null)
    setIsStreaming(true)
    setStreamingContent('')
    streamingContentRef.current = ''
    setStreamStats(null)
    streamTimerRef.current = { startedAt: performance.now(), firstTokenAt: null }

    const controller = new AbortController()
    abortRef.current = controller
    try {
      await sendChatMessage(
        { model: selectedModel, messages: msgs },
        {
          onChunk: (delta) => {
            if (abortRef.current !== controller) return
            if (streamTimerRef.current.firstTokenAt === null) {
              streamTimerRef.current.firstTokenAt = performance.now()
            }
            setStreamingContent((prev) => {
              const next = prev + delta
              streamingContentRef.current = next
              return next
            })
          },
          onComplete: (full) => {
            if (abortRef.current !== controller) return
            const now = performance.now()
            const { startedAt, firstTokenAt } = streamTimerRef.current
            setStreamStats({
              ttft: firstTokenAt != null ? firstTokenAt - startedAt : now - startedAt,
              total: now - startedAt,
            })
            const final = buildAssistantReply(msgs, full)
            if (final) {
              updateMessages(convId, final)
            }
            streamingContentRef.current = ''
            setStreamingContent('')
            setIsStreaming(false)
            abortRef.current = null
          },
          onError: (err) => {
            if (abortRef.current !== controller) return
            setError(err.message)
            setIsStreaming(false)
            const final = buildAssistantReply(msgs, streamingContentRef.current)
            if (final) {
              updateMessages(convId, final)
            }
            streamingContentRef.current = ''
            setStreamingContent('')
            abortRef.current = null
          },
        },
        controller.signal
      )
    } catch { /* handled by callbacks */ }
  }

  const abort = () => {
    abortRef.current?.abort()
    // Synchronously clear UI state to prevent cross-conversation flashes
    setIsStreaming(false)
    setStreamingContent('')
    streamingContentRef.current = ''
    // Notice: We intentionally do NOT set abortRef.current = null here.
    // This allows the AbortError handlers in fetch/stream to fire onComplete,
    // which gracefully saves the partial message to the current conversation's history!
  }

  const resetState = () => {
    abort()
    setError(null)
    setStreamStats(null)
  }

  return {
    isStreaming,
    streamingContent,
    error,
    setError,
    streamStats,
    streamTimerRef,
    abortRef,
    doSend,
    abort,
    resetState,
  }
}
