'use client'

import * as React from 'react'
import { Button, Card, CardContent, Select, SelectOption, Textarea, Label, Progress, useToast } from '@/components/ui'
import { useVideoModels } from '@/lib/hooks/use-models'
import { Play, Square } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { RefImageUpload } from './ref-image-upload'
import { useVideoGeneration } from './use-video-generation'

const RESOLUTION_OPTIONS = [
  { value: '480p', label: '480p' },
  { value: '720p', label: '720p' },
]

export function VideoPanel() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { models: videoModels, isLoading: modelsLoading } = useVideoModels()
  const [selectedModel, setSelectedModel] = React.useState('')
  const [prompt, setPrompt] = React.useState('')
  const [ratio, setRatio] = React.useState('3:2')
  const [duration, setDuration] = React.useState('6')
  const [resolution, setResolution] = React.useState('480p')
  const [style, setStyle] = React.useState('normal')
  const [refImage, setRefImage] = React.useState<string | null>(null)

  const RATIO_OPTIONS = [
    { value: '3:2', label: t.function.ratioLandscape },
    { value: '2:3', label: t.function.ratioPortrait },
    { value: '16:9', label: t.function.ratioWidescreen },
    { value: '9:16', label: t.function.ratioVertical },
    { value: '1:1', label: t.function.ratioSquare },
  ]

  const DURATION_OPTIONS = Array.from({ length: 25 }, (_, i) => ({
    value: String(i + 6),
    label: t.function.durationSeconds.replace('{n}', String(i + 6)),
  }))

  const STYLE_OPTIONS = [
    { value: 'normal', label: t.function.styleNormal },
    { value: 'fun', label: t.function.styleFun },
    { value: 'spicy', label: t.function.styleSpicy },
    { value: 'custom', label: t.function.styleCustom },
  ]

  React.useEffect(() => {
    if (videoModels.length > 0 && !selectedModel) setSelectedModel(videoModels[0])
  }, [videoModels, selectedModel])

  const { loading, progress, error, result, elapsed, handleGenerate, handleStop, formatElapsed } =
    useVideoGeneration({ selectedModel, prompt, ratio, duration, resolution, style, refImage })

  const handleDownload = async () => {
    const url = result?.video_url
    if (!url || !/^https?:\/\//i.test(url)) return

    try {
      const a = document.createElement('a')
      a.href = url
      a.download = `grokforge-video-${Date.now()}.mp4`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
    } catch {
      toast({ title: t.common.error, description: t.function.downloadFailed, variant: 'destructive' })
    }
  }

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      {/* Left: Input Area */}
      <Card className="lg:col-span-1">
        <CardContent className="p-6 space-y-4">
          {/* Model */}
          <div className="space-y-2">
            <Label>{t.function.model}</Label>
            <Select value={selectedModel} onChange={(e) => setSelectedModel(e.target.value)} disabled={modelsLoading || videoModels.length === 0}>
              {modelsLoading ? (
                <SelectOption value="">{t.function.loadingModels}</SelectOption>
              ) : videoModels.length === 0 ? (
                <SelectOption value="">{t.function.noModelsAvailable}</SelectOption>
              ) : videoModels.map((m) => <SelectOption key={m} value={m}>{m}</SelectOption>)}
            </Select>
          </div>
          {/* Prompt */}
          <div className="space-y-2">
            <Label htmlFor="video-prompt">{t.function.prompt}</Label>
            <Textarea id="video-prompt" value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t.function.describeVideo} className="min-h-[100px]" />
          </div>
          {/* Aspect Ratio */}
          <div className="space-y-2">
            <Label htmlFor="ratio">{t.function.aspectRatio}</Label>
            <Select value={ratio} onChange={(e) => setRatio(e.target.value)}>
              {RATIO_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Duration */}
          <div className="space-y-2">
            <Label htmlFor="duration">{t.function.duration}</Label>
            <Select value={duration} onChange={(e) => setDuration(e.target.value)}>
              {DURATION_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Resolution */}
          <div className="space-y-2">
            <Label htmlFor="resolution">{t.function.resolution}</Label>
            <Select value={resolution} onChange={(e) => setResolution(e.target.value)}>
              {RESOLUTION_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Style */}
          <div className="space-y-2">
            <Label htmlFor="style">{t.function.style}</Label>
            <Select value={style} onChange={(e) => setStyle(e.target.value)}>
              {STYLE_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Reference Image */}
          <div className="space-y-2">
            <Label>{t.function.referenceImage}</Label>
            <RefImageUpload image={refImage} onImageChange={setRefImage} />
          </div>
          {/* Error */}
          {error && <div className="text-sm text-destructive bg-destructive/8 p-3 rounded-md">{error}</div>}
          {/* Generate/Stop */}
          <div className="flex gap-2">
            {loading ? (
              <Button className="flex-1" variant="outline" onClick={handleStop}>
                <Square className="h-4 w-4 mr-2" />{t.function.stop}
              </Button>
            ) : (
              <Button className="flex-1" onClick={handleGenerate} disabled={!selectedModel}>
                <Play className="h-4 w-4 mr-2" />{t.function.generate}
              </Button>
            )}
          </div>
        </CardContent>
      </Card>
      {/* Right: Results Area */}
      <Card className="lg:col-span-2">
        <CardContent className="p-6">
          <h3 className="font-semibold mb-4">{t.function.videoPreview}</h3>
          {loading && (
            <div className="space-y-3 mb-6">
              <div className="flex justify-between text-sm">
                <span className="text-muted">{t.function.generating}</span>
                <span>{progress}%</span>
              </div>
              <Progress value={progress} className="h-2" />
              <div className="flex justify-between text-xs text-muted">
                <span>{t.function.elapsed} {formatElapsed(elapsed)}</span>
                <span>{ratio} | {duration}{t.function.secondsShort} | {resolution} | {style}</span>
              </div>
            </div>
          )}
          {result?.video_url ? (
            <div className="space-y-4">
              <video src={result.video_url} controls className="w-full rounded-lg bg-black" autoPlay />
              <div className="flex justify-end">
                <Button variant="outline" size="sm" onClick={() => void handleDownload()}>{t.function.downloadVideo}</Button>
              </div>
            </div>
          ) : !loading && (
            <div className="flex items-center justify-center h-64 text-muted border-2 border-dashed rounded-lg">
              {t.function.noVideo}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
