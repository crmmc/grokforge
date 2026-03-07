'use client'

import * as React from 'react'
import { Loader2, ChevronRight } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { parseThinkContent } from '@/lib/think-parser'

const MarkdownRenderer = React.lazy(async () => {
  const mod = await import('./markdown-renderer')
  return { default: mod.MarkdownRenderer }
})

const bubbleCls = (isUser: boolean) =>
  `max-w-[80%] rounded-lg px-4 py-2 whitespace-pre-wrap break-words ${isUser ? 'bg-primary text-primary-foreground' : 'bg-[rgba(0,0,0,0.04)]'}`

export { bubbleCls }

/** Render content: plain text while streaming, Markdown after completion. */
export function ContentRenderer({ content, streaming }: { content: string; streaming?: boolean }) {
  if (streaming) {
    return <span className="whitespace-pre-wrap">{content}</span>
  }

  if (!shouldRenderMarkdown(content)) {
    return <span className="whitespace-pre-wrap">{content}</span>
  }

  return (
    <React.Suspense fallback={<span className="whitespace-pre-wrap">{content}</span>}>
      <MarkdownRenderer content={content} />
    </React.Suspense>
  )
}

export function ElapsedTimer({ startedAt, label }: { startedAt: number; label: string }) {
  const [elapsed, setElapsed] = React.useState(0)

  React.useEffect(() => {
    setElapsed(performance.now() - startedAt)

    const timerId = window.setInterval(() => {
      setElapsed(performance.now() - startedAt)
    }, 100)

    return () => window.clearInterval(timerId)
  }, [startedAt])

  return <span>{label} {(elapsed / 1000).toFixed(1)}s</span>
}

export function AssistantBubble({
  content, streaming, startedAt, stats,
}: {
  content: string
  streaming?: boolean
  startedAt?: number
  stats?: { ttft: number; total: number }
}) {
  const { t } = useTranslation()
  const [open, setOpen] = React.useState(false)
  const thinkRef = React.useRef<HTMLDivElement>(null)
  const parsed = parseThinkContent(content)

  // Auto-scroll thinking container to bottom during streaming
  React.useEffect(() => {
    if (parsed.isThinking && thinkRef.current) {
      thinkRef.current.scrollTop = thinkRef.current.scrollHeight
    }
  }, [parsed.isThinking, parsed.thinking])

  if (!content && streaming) {
    return (
      <div className={bubbleCls(false)}>
        <span className="flex items-center gap-2 text-muted">
          <Loader2 className="h-3 w-3 animate-spin" />
          {startedAt != null
            ? <ElapsedTimer startedAt={startedAt} label={t.function.waitingResponse} />
            : t.function.waitingResponse}
        </span>
      </div>
    )
  }

  const statsBar = !streaming && stats ? (
    <div className="px-4 pb-1.5 flex justify-end">
      <span className="text-xs text-muted/60">
        {t.function.firstToken} {(stats.ttft / 1000).toFixed(1)}s / {t.function.totalTime} {(stats.total / 1000).toFixed(1)}s
      </span>
    </div>
  ) : null

  if (!parsed.thinking && !parsed.isThinking) {
    return (
      <div className="max-w-[80%] rounded-lg bg-[rgba(0,0,0,0.04)] overflow-hidden break-words">
        <div className="px-4 py-2"><ContentRenderer content={parsed.answer || content} streaming={streaming} /></div>
        {statsBar}
      </div>
    )
  }

  return (
    <div className="max-w-[80%] rounded-lg bg-[rgba(0,0,0,0.04)] overflow-hidden">
      {parsed.isThinking ? (
        <div className="border-b border-[rgba(0,0,0,0.06)]">
          <div className="flex items-center gap-1.5 px-4 py-1.5 text-muted text-xs">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t.function.thinking}
          </div>
          {parsed.thinking && (
            <div ref={thinkRef} className="px-4 pb-2 text-xs text-muted/80 whitespace-pre-wrap max-h-[200px] overflow-y-auto border-t border-border/30">
              {parsed.thinking}
            </div>
          )}
        </div>
      ) : (
        <div className="border-b border-[rgba(0,0,0,0.06)]">
          <button
            type="button"
            onClick={() => setOpen(!open)}
            className="w-full flex items-center gap-1.5 px-4 py-1.5 text-muted text-xs hover:text-foreground/70 transition-colors"
            aria-expanded={open}
            aria-label={open ? t.common.collapse : t.common.expand}
          >
            <ChevronRight className={`h-3 w-3 transition-transform ${open ? 'rotate-90' : ''}`} />
            {t.function.thinkingProcess}
          </button>
          {open && (
            <div className="px-4 py-2 text-xs text-muted/80 whitespace-pre-wrap max-h-[200px] overflow-y-auto border-t border-border/30">
              {parsed.thinking}
            </div>
          )}
        </div>
      )}
      {parsed.answer && <div className="px-4 py-2 break-words overflow-hidden"><ContentRenderer content={parsed.answer} streaming={streaming} /></div>}
      {statsBar}
    </div>
  )
}

function shouldRenderMarkdown(content: string): boolean {
  return /```|`[^`]+`|^#{1,6}\s|\[[^\]]+\]\([^\)]+\)|^>\s|^[-*+]\s|^\d+\.\s|\*\*|__|~~|\||<[^>]+>/m.test(content)
}
