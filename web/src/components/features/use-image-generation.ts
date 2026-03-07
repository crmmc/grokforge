import * as React from 'react'
import { generateImage, getApiKey, type GeneratedImage, type ImageGenerateParams } from '@/lib/function-api'
import { useTranslation } from '@/lib/i18n/context'

interface UseImageGenerationOptions {
  selectedModel: string
  prompt: string
  size: string
  count: string
  format: string
  editMode: boolean
  editImage: string | null
}

export function useImageGeneration({
  selectedModel, prompt, size, count, format, editMode, editImage,
}: UseImageGenerationOptions) {
  const { t } = useTranslation()
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [images, setImages] = React.useState<GeneratedImage[]>([])
  const abortRef = React.useRef<AbortController | null>(null)

  const handleGenerate = async () => {
    if (!prompt.trim()) { setError(t.function.enterPrompt); return }
    if (!getApiKey()) { setError(t.function.configureApiKey); return }

    setLoading(true)
    setError(null)
    const controller = new AbortController()
    abortRef.current = controller

    try {
      const params: ImageGenerateParams = {
        prompt: prompt.trim(),
        model: editMode ? 'grok-imagine-1.0-edit' : selectedModel,
        size,
        n: parseInt(count),
        response_format: format as 'url' | 'b64_json' | 'base64',
      }
      if (editMode && editImage) params.image = editImage

      const result = await generateImage(params, controller.signal)
      if (abortRef.current === controller) setImages((prev) => [...result.data, ...prev])
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') { /* cancelled */ }
      else if (abortRef.current === controller) {
        setError(err instanceof Error ? err.message : t.function.generationFailed)
      }
    } finally {
      if (abortRef.current === controller) abortRef.current = null
      setLoading(false)
    }
  }

  const clearImages = () => setImages([])

  return { loading, error, images, count, handleGenerate, clearImages }
}
