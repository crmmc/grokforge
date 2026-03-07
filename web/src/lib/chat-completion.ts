export interface ChatContentTextBlock {
  type: 'text'
  text: string
}

export interface ChatContentImageBlock {
  type: 'image_url'
  image_url: {
    url: string
  }
}

export type ChatContentBlock = ChatContentTextBlock | ChatContentImageBlock

export interface ChatCompletionMessageRequest {
  role: 'user' | 'assistant' | 'system'
  content: string | ChatContentBlock[]
}

export interface ChatCompletionResponse {
  id: string
  created: number
  choices?: Array<{
    message?: {
      content?: string | null
    }
  }>
}

interface StreamCallbacks {
  onChunk?: (delta: string) => void
  onComplete?: (fullContent: string) => void
  onError?: (error: Error) => void
}

export function buildImageMessages(prompt: string, image?: string): ChatCompletionMessageRequest[] {
  if (!image) {
    return [{ role: 'user', content: prompt }]
  }

  return [{
    role: 'user',
    content: [
      { type: 'image_url', image_url: { url: toImageDataURI(image) } },
      { type: 'text', text: prompt },
    ],
  }]
}

export function buildVideoMessages(prompt: string, image?: string): ChatCompletionMessageRequest[] {
  return buildImageMessages(prompt, image)
}

export function buildImageConfig(params: {
  n?: number
  size?: string
  response_format?: 'url' | 'b64_json' | 'base64'
}): Record<string, unknown> {
  const imageConfig: Record<string, unknown> = {}
  if (params.n) imageConfig.n = params.n
  if (params.size) imageConfig.size = params.size
  if (params.response_format) imageConfig.response_format = params.response_format
  return imageConfig
}

export function buildVideoConfig(params: {
  aspect_ratio?: '16:9' | '9:16' | '1:1' | '2:3' | '3:2'
  duration?: number
  resolution?: '480p' | '720p'
  style?: 'fun' | 'normal' | 'spicy' | 'custom'
}): Record<string, unknown> {
  const videoConfig: Record<string, unknown> = {}
  if (params.aspect_ratio) videoConfig.aspect_ratio = params.aspect_ratio
  if (params.duration) videoConfig.video_length = params.duration
  if (params.resolution) videoConfig.resolution_name = params.resolution
  if (params.style) videoConfig.preset = params.style
  return videoConfig
}

export function extractAssistantContent(response: ChatCompletionResponse): string {
  return response.choices?.[0]?.message?.content || ''
}

export function parseImageMarkdown(content: string): Array<{ url?: string; b64_json?: string }> {
  const matches = content.matchAll(/!\[image\]\(([^)]+)\)/g)
  const images: Array<{ url?: string; b64_json?: string }> = []
  for (const match of matches) {
    const value = match[1]
    const dataUriMatch = value.match(/^data:image\/[^;]+;base64,(.+)$/)
    if (dataUriMatch) {
      images.push({ b64_json: dataUriMatch[1] })
      continue
    }
    images.push({ url: value })
  }
  return images
}

export function parseVideoMarkdown(content: string): string {
  const match = content.match(/\[video\]\(([^)]+)\)/)
  return match?.[1] || ''
}

export async function readChatCompletionStream(
  response: Response,
  callbacks: StreamCallbacks,
  createError: (status: number, code: string, message: string) => Error
): Promise<string> {
  const reader = response.body?.getReader()
  if (!reader) {
    const error = createError(500, 'NO_BODY', 'Response body is empty')
    callbacks.onError?.(error)
    throw error
  }

  const decoder = new TextDecoder()
  let fullContent = ''
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''

      for (const line of lines) {
        fullContent = appendStreamDelta(line, fullContent, callbacks, createError)
      }
    }

    if (buffer.trim().startsWith('data: ')) {
      fullContent = appendStreamDelta(buffer.trim(), fullContent, callbacks, createError)
    }

    callbacks.onComplete?.(fullContent)
    return fullContent
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') {
      callbacks.onComplete?.(fullContent)
      return fullContent
    }
    const streamError = error instanceof Error ? error : new Error(String(error))
    callbacks.onError?.(streamError)
    throw streamError
  } finally {
    reader.releaseLock()
  }
}

function appendStreamDelta(
  line: string,
  fullContent: string,
  callbacks: StreamCallbacks,
  createError: (status: number, code: string, message: string) => Error
): string {
  const trimmed = line.trim()
  if (!trimmed.startsWith('data: ')) return fullContent

  const data = trimmed.slice(6)
  if (data === '[DONE]') return fullContent

  try {
    const parsed = JSON.parse(data)
    if (parsed.error) {
      const error = createError(500, parsed.error.code || 'STREAM_ERROR', parsed.error.message || 'Stream error')
      callbacks.onError?.(error)
      return fullContent
    }
    const delta = parsed.choices?.[0]?.delta?.content
    if (!delta) return fullContent
    callbacks.onChunk?.(delta)
    return fullContent + delta
  } catch {
    return fullContent
  }
}

function toImageDataURI(base64: string): string {
  return `data:image/png;base64,${base64}`
}
