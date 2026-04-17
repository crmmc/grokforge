'use client'

import * as React from 'react'
import { Button, Card, CardContent, Select, SelectOption, Textarea, Label, Progress } from '@/components/ui'
import { ImageGrid } from './image-grid'
import { useImageEditModels, useImageModels } from '@/lib/hooks/use-models'
import { Loader2 } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { RefImageUpload } from './ref-image-upload'
import { useImageGeneration } from './use-image-generation'

const SIZE_OPTIONS = [
  { value: '1024x1024', label: '1024x1024', descKey: 'sizeSquare' as const },
  { value: '1280x720', label: '1280x720', descKey: 'sizeLandscape' as const },
  { value: '720x1280', label: '720x1280', descKey: 'sizePortrait' as const },
  { value: '1792x1024', label: '1792x1024', descKey: 'sizeWide' as const },
  { value: '1024x1792', label: '1024x1792', descKey: 'sizeTall' as const },
]

const FORMAT_OPTIONS = [
  { value: 'b64_json', label: 'Base64 JSON' },
  { value: 'base64', label: 'Base64' },
]

export function ImaginePanel() {
  const { t } = useTranslation()
  const { models: imageModels, isLoading: modelsLoading } = useImageModels()
  const { models: imageEditModels, isLoading: editModelsLoading } = useImageEditModels()
  const [selectedModel, setSelectedModel] = React.useState('')
  const [prompt, setPrompt] = React.useState('')
  const [size, setSize] = React.useState('1024x1024')
  const [count, setCount] = React.useState('1')
  const [format, setFormat] = React.useState('b64_json')
  const [editMode, setEditMode] = React.useState(false)
  const [editImage, setEditImage] = React.useState<string | null>(null)

  const COUNT_OPTIONS = Array.from({ length: 10 }, (_, i) => ({
    value: String(i + 1),
    label: i > 0 ? t.function.imageCountPlural.replace('{n}', String(i + 1)) : t.function.imageCount.replace('{n}', String(i + 1)),
  }))
  const activeModels = editMode ? imageEditModels : imageModels
  const modelsLoadingState = editMode ? editModelsLoading : modelsLoading

  React.useEffect(() => {
    if (activeModels.length === 0) {
      if (selectedModel !== '') {
        setSelectedModel('')
      }
      return
    }
    if (!activeModels.includes(selectedModel)) {
      setSelectedModel(activeModels[0])
    }
  }, [activeModels, selectedModel])

  const handleImageChange = (base64: string | null) => {
    setEditImage(base64)
    setEditMode(!!base64)
  }

  const { loading, error, images, handleGenerate, clearImages } =
    useImageGeneration({ selectedModel, prompt, size, count, format, editMode, editImage })

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      {/* Left: Input Area */}
      <Card className="lg:col-span-1">
        <CardContent className="p-6 space-y-4">
          {/* Model */}
          <div className="space-y-2">
            <Label>{t.function.model}</Label>
            <Select value={selectedModel} onChange={(e) => setSelectedModel(e.target.value)} disabled={modelsLoadingState || activeModels.length === 0}>
              {modelsLoadingState ? (
                <SelectOption value="">{t.function.loadingModels}</SelectOption>
              ) : activeModels.length === 0 ? (
                <SelectOption value="">{t.function.noModelsAvailable}</SelectOption>
              ) : activeModels.map((m) => <SelectOption key={m} value={m}>{m}</SelectOption>)}
            </Select>
          </div>
          {/* Prompt */}
          <div className="space-y-2">
            <Label htmlFor="prompt">{t.function.prompt}</Label>
            <Textarea id="prompt" value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder={t.function.describeImage} className="min-h-[120px]" />
          </div>
          {/* Size */}
          <div className="space-y-2">
            <Label htmlFor="size">{t.function.size}</Label>
            <Select value={size} onChange={(e) => setSize(e.target.value)}>
              {SIZE_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label} ({t.function[opt.descKey]})</SelectOption>)}
            </Select>
          </div>
          {/* Count */}
          <div className="space-y-2">
            <Label htmlFor="count">{t.function.count}</Label>
            <Select value={count} onChange={(e) => setCount(e.target.value)}>
              {COUNT_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Format */}
          <div className="space-y-2">
            <Label htmlFor="format">{t.function.format}</Label>
            <Select value={format} onChange={(e) => setFormat(e.target.value)}>
              {FORMAT_OPTIONS.map((opt) => <SelectOption key={opt.value} value={opt.value}>{opt.label}</SelectOption>)}
            </Select>
          </div>
          {/* Image Edit Upload */}
          <div className="space-y-2">
            <Label>{t.function.editMode}</Label>
            <RefImageUpload image={editImage} onImageChange={handleImageChange} label={t.function.editSource} maxHeight="max-h-32" />
          </div>
          {/* Error */}
          {error && <div className="text-sm text-destructive bg-destructive/8 p-3 rounded-md">{error}</div>}
          {/* Generate Button */}
          <Button className="w-full" onClick={handleGenerate} disabled={loading || !selectedModel}>
            {loading ? (
              <><Loader2 className="h-4 w-4 mr-2 animate-spin" />{t.function.generating}</>
            ) : t.function.generate}
          </Button>
        </CardContent>
      </Card>
      {/* Right: Results Area */}
      <Card className="lg:col-span-2">
        <CardContent className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-semibold">{t.function.generatedImages}</h3>
            {images.length > 0 && (
              <Button variant="outline" size="sm" onClick={clearImages}>{t.common.clearAll}</Button>
            )}
          </div>
          {loading && (
            <div className="mb-4">
              <Progress indeterminate className="h-2" />
              <p className="text-sm text-muted mt-2 text-center">{t.function.generatingImages.replace('{n}', count)}</p>
            </div>
          )}
          <ImageGrid images={images} />
        </CardContent>
      </Card>
    </div>
  )
}
