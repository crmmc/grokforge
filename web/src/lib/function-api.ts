// Function API client for Imagine/Video generation

import {
  buildImageConfig,
  buildImageMessages,
  buildVideoConfig,
  buildVideoMessages,
  extractAssistantContent,
  parseImageMarkdown,
  parseVideoMarkdown,
  readChatCompletionStream,
  type ChatCompletionMessageRequest,
  type ChatCompletionResponse,
  type ChatContentBlock,
} from './chat-completion'

const API_KEY_STORAGE_KEY = 'grokforge_api_key'

export function getApiKey(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(API_KEY_STORAGE_KEY)
}

export function setApiKey(key: string): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(API_KEY_STORAGE_KEY, key)
}

export function clearApiKey(): void {
  if (typeof window === 'undefined') return
  localStorage.removeItem(API_KEY_STORAGE_KEY)
}

export interface ModelEntry {
  id: string
  object: string
  created: number
  owned_by: string
  type?: string
}

export interface ModelsResponse {
  object: string
  data: ModelEntry[]
}

export async function fetchModels(): Promise<ModelsResponse> {
  const response = await fetchWithAuth('/v1/models')
  if (!response.ok) {
    const data = await response.json().catch(() => null)
    const detail = data?.error
    throw new FunctionAPIError(response.status, detail?.code || 'UNKNOWN', detail?.message || 'Failed to fetch models')
  }
  return response.json()
}

export interface ImageGenerateParams {
  prompt: string
  model?: string
  size?: string
  n?: number
  response_format?: 'url' | 'b64_json' | 'base64'
  image?: string // base64 image for edit mode
}

export interface VideoGenerateParams {
  prompt: string
  model?: string
  aspect_ratio?: '16:9' | '9:16' | '1:1' | '2:3' | '3:2'
  duration?: number
  resolution?: '480p' | '720p'
  style?: 'fun' | 'normal' | 'spicy' | 'custom'
  image?: string // reference image base64
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
}

export interface ImageGenerateResult {
  created: number
  data: GeneratedImage[]
}

export interface VideoGenerateResult {
  id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  progress?: number
  video_url?: string
  error?: string
}

export class FunctionAPIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message)
    this.name = 'FunctionAPIError'
  }
}

async function fetchWithAuth(url: string, options: RequestInit = {}): Promise<Response> {
  const apiKey = getApiKey()
  if (!apiKey) {
    throw new FunctionAPIError(401, 'NO_API_KEY', 'API Key not configured')
  }

  return fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${apiKey}`,
      ...options.headers,
    },
  })
}

export async function generateImage(params: ImageGenerateParams, signal?: AbortSignal): Promise<ImageGenerateResult> {
  const model = params.model?.trim()
  if (!model) {
    throw new FunctionAPIError(400, 'MODEL_REQUIRED', 'Model is required')
  }
  const response = await requestChatCompletion({
    model,
    messages: buildImageMessages(params.prompt, params.image),
    image_config: buildImageConfig(params),
  }, signal)
  const content = extractAssistantContent(response)
  const data = parseImageMarkdown(content)
  if (data.length === 0) {
    throw new FunctionAPIError(500, 'INVALID_IMAGE_RESPONSE', 'Image generation returned no images')
  }

  return {
    created: response.created || Math.floor(Date.now() / 1000),
    data,
  }
}

export interface StreamCallbacks {
  onProgress?: (progress: number) => void
  onComplete?: (result: VideoGenerateResult) => void
  onError?: (error: Error) => void
}

// Chat types and streaming
export interface ChatMessage {
  id?: string
  role: 'user' | 'assistant' | 'system'
  content: string
}

export interface ChatStreamCallbacks {
  onChunk?: (delta: string) => void
  onComplete?: (fullContent: string) => void
  onError?: (error: Error) => void
}

const BASE64_IMAGE_RE = /!\[[^\]]*\]\((data:image\/[^;]+;base64,[^)]+)\)/g

function prepareMessagesForSend(messages: ChatMessage[]): ChatCompletionMessageRequest[] {
  return messages.map((msg) => {
    if (msg.role !== 'assistant') {
      return { role: msg.role, content: msg.content }
    }
    const images: string[] = []
    const text = msg.content.replace(BASE64_IMAGE_RE, (_, dataUri: string) => {
      images.push(dataUri)
      return ''
    }).trim()

    if (images.length === 0) {
      return { role: msg.role, content: msg.content }
    }

    const blocks: ChatContentBlock[] = []
    if (text) blocks.push({ type: 'text', text })
    for (const uri of images) {
      blocks.push({ type: 'image_url', image_url: { url: uri } })
    }
    return { role: 'assistant', content: blocks }
  })
}

export async function sendChatMessage(
  params: { model: string; messages: ChatMessage[] },
  callbacks: ChatStreamCallbacks,
  signal?: AbortSignal
): Promise<string> {
  let response: Response
  try {
    response = await fetchWithAuth('/v1/chat/completions', {
      method: 'POST',
      body: JSON.stringify({
        model: params.model,
        messages: prepareMessagesForSend(params.messages),
        stream: true,
      }),
      signal,
    })
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      callbacks.onComplete?.('')
      return ''
    }
    const error = err instanceof Error ? err : new Error(String(err))
    callbacks.onError?.(error)
    throw error
  }

  if (!response.ok) {
    const data = await response.json().catch(() => null)
    const detail = data?.error
    const error = new FunctionAPIError(
      response.status,
      detail?.code || 'UNKNOWN',
      detail?.message || 'Chat request failed'
    )
    callbacks.onError?.(error)
    throw error
  }

  try {
    return await readChatCompletionStream(
      response,
      callbacks,
      (status, code, message) => new FunctionAPIError(status, code, message)
    )
  } catch (err) {
    throw err instanceof Error ? err : new Error(String(err))
  }
}

export async function generateVideo(
  params: VideoGenerateParams,
  callbacks?: StreamCallbacks,
  signal?: AbortSignal
): Promise<VideoGenerateResult> {
  try {
    const model = params.model?.trim()
    if (!model) {
      throw new FunctionAPIError(400, 'MODEL_REQUIRED', 'Model is required')
    }
    const response = await requestChatCompletion({
      model,
      messages: buildVideoMessages(params.prompt, params.image),
      video_config: buildVideoConfig(params),
    }, signal)
    const content = extractAssistantContent(response)
    const videoURL = parseVideoMarkdown(content)
    if (!videoURL) {
      throw new FunctionAPIError(500, 'INVALID_VIDEO_RESPONSE', 'Video generation returned no URL')
    }

    const result: VideoGenerateResult = {
      id: response.id,
      status: 'completed',
      progress: 100,
      video_url: videoURL,
    }
    callbacks?.onProgress?.(100)
    callbacks?.onComplete?.(result)
    return result
  } catch (error) {
    const err = error instanceof Error ? error : new Error(String(error))
    callbacks?.onError?.(err)
    throw err
  }
}

async function requestChatCompletion(body: Record<string, unknown>, signal?: AbortSignal): Promise<ChatCompletionResponse> {
  const response = await fetchWithAuth('/v1/chat/completions', {
    method: 'POST',
    body: JSON.stringify({ ...body, stream: false }),
    signal,
  })
  if (!response.ok) {
    const data = await response.json().catch(() => null)
    const detail = data?.error
    throw new FunctionAPIError(response.status, detail?.code || 'UNKNOWN', detail?.message || 'Request failed')
  }
  return response.json()
}
