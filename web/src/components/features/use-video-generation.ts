import * as React from 'react'
import { generateVideo, getApiKey, type VideoGenerateParams, type VideoGenerateResult } from '@/lib/function-api'
import { useTranslation } from '@/lib/i18n/context'

interface UseVideoGenerationOptions {
  selectedModel: string
  prompt: string
  ratio: string
  duration: string
  resolution: string
  style: string
  refImage: string | null
}

export function useVideoGeneration({
  selectedModel, prompt, ratio, duration, resolution, style, refImage,
}: UseVideoGenerationOptions) {
  const { t } = useTranslation()
  const [loading, setLoading] = React.useState(false)
  const [progress, setProgress] = React.useState(0)
  const [error, setError] = React.useState<string | null>(null)
  const [result, setResult] = React.useState<VideoGenerateResult | null>(null)
  const [startTime, setStartTime] = React.useState<number | null>(null)
  const [elapsed, setElapsed] = React.useState(0)
  const abortRef = React.useRef<AbortController | null>(null)

  // Elapsed time timer
  React.useEffect(() => {
    if (!loading || !startTime) return
    const interval = setInterval(() => {
      setElapsed(Math.floor((Date.now() - startTime) / 1000))
    }, 1000)
    return () => clearInterval(interval)
  }, [loading, startTime])

  const handleGenerate = async () => {
    if (!prompt.trim()) { setError(t.function.enterPrompt); return }
    if (!getApiKey()) { setError(t.function.configureApiKey); return }
    if (!selectedModel) { setError(t.function.noModelsAvailable); return }

    setLoading(true)
    setError(null)
    setProgress(0)
    setResult(null)
    setStartTime(Date.now())
    setElapsed(0)
    const controller = new AbortController()
    abortRef.current = controller

    try {
      const params: VideoGenerateParams = {
        prompt: prompt.trim(),
        model: selectedModel,
        aspect_ratio: ratio as VideoGenerateParams['aspect_ratio'],
        duration: parseInt(duration),
        resolution: resolution as '480p' | '720p',
        style: style as VideoGenerateParams['style'],
      }
      if (refImage) params.image = refImage

      const videoResult = await generateVideo(params, {
        onProgress: (p) => { if (abortRef.current === controller) setProgress(p) },
        onComplete: (r) => { if (abortRef.current === controller) setResult(r) },
        onError: (err) => { if (abortRef.current === controller) setError(err.message) },
      }, controller.signal)

      if (abortRef.current === controller) setResult(videoResult)
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') { /* cancelled */ }
      else if (abortRef.current === controller) {
        setError(err instanceof Error ? err.message : t.function.generationFailed)
      }
    } finally {
      if (abortRef.current === controller) abortRef.current = null
      setLoading(false)
      setStartTime(null)
    }
  }

  const handleStop = () => {
    abortRef.current?.abort()
    abortRef.current = null
    setLoading(false)
    setStartTime(null)
  }

  const formatElapsed = (seconds: number): string => {
    const mins = Math.floor(seconds / 60)
    const secs = seconds % 60
    return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`
  }

  return { loading, progress, error, result, elapsed, handleGenerate, handleStop, formatElapsed }
}
