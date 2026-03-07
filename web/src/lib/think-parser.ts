/** Parse <think> blocks from model responses into structured content. */

export interface ParsedContent {
  /** Combined thinking text (all <think> blocks joined), null if none */
  thinking: string | null
  /** The answer text with think blocks stripped */
  answer: string
  /** True when a <think> tag is open without a matching </think> (streaming) */
  isThinking: boolean
}

// Grok internal tool usage cards that leak into responses
const XAI_TAG_RE = /<xai:tool_usage_card[\s\S]*?<\/xai:tool_usage_card>/g
const THINK_BLOCK_RE = /<think>([\s\S]*?)<\/think>/g

export function parseThinkContent(content: string): ParsedContent {
  const cleaned = content.replace(XAI_TAG_RE, '')

  const openCount = (cleaned.match(/<think>/g) || []).length
  if (openCount === 0) return { thinking: null, answer: cleaned.trim(), isThinking: false }

  const closeCount = (cleaned.match(/<\/think>/g) || []).length
  const isThinking = openCount > closeCount

  // Extract completed <think>...</think> blocks
  const thinkParts: string[] = []
  let remainder = cleaned.replace(THINK_BLOCK_RE, (_, body: string) => {
    const t = body.trim()
    if (t) thinkParts.push(t)
    return ''
  })

  // If still in an open <think>, capture the partial content
  if (isThinking) {
    const idx = remainder.indexOf('<think>')
    if (idx !== -1) {
      const partial = remainder.substring(idx + 7).trim()
      if (partial) thinkParts.push(partial)
      remainder = remainder.substring(0, idx)
    }
  }

  return {
    thinking: thinkParts.length > 0 ? thinkParts.join('\n\n') : null,
    answer: remainder.trim(),
    isThinking,
  }
}
