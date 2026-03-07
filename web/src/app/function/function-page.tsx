'use client'

import * as React from 'react'
import dynamic from 'next/dynamic'
import { Tabs, TabsList, TabsTrigger, TabsContent, Button, Input, ConfirmProvider } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { getApiKey, setApiKey } from '@/lib/function-api'
import { useQueryClient } from '@tanstack/react-query'
import { modelKeys } from '@/lib/hooks/use-models'
import { Key } from 'lucide-react'

const ChatPanel = dynamic(
  () => import('@/components/features/chat-panel').then((mod) => mod.ChatPanel),
  { loading: () => <PanelSkeleton /> }
)

const ImaginePanel = dynamic(
  () => import('@/components/features/imagine-panel').then((mod) => mod.ImaginePanel),
  { loading: () => <PanelSkeleton /> }
)

const VideoPanel = dynamic(
  () => import('@/components/features/video-panel').then((mod) => mod.VideoPanel),
  { loading: () => <PanelSkeleton /> }
)

export default function FunctionPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [apiKeyValue, setApiKeyValue] = React.useState('')

  React.useEffect(() => {
    const key = getApiKey()
    if (key) setApiKeyValue(key)
  }, [])

  const handleApply = () => {
    setApiKey(apiKeyValue.trim())
    queryClient.invalidateQueries({ queryKey: modelKeys.all })
  }

  return (
    <ConfirmProvider>
      <div className="flex-1 flex flex-col min-h-0 space-y-6">
        <div className="flex flex-col gap-1 shrink-0">
          <h1 className="text-3xl font-bold tracking-tight">{t.function.title}</h1>
          <p className="text-muted text-sm">
            {t.function.description}
          </p>
        </div>

        <Tabs defaultValue="chat" className="flex-1 flex flex-col min-h-0 min-w-0 pb-4 md:pb-8">
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 mb-4 shrink-0">
            <TabsList className="w-full sm:w-auto flex-wrap h-auto">
              <TabsTrigger value="chat" className="flex-1 sm:flex-none">{t.function.chat}</TabsTrigger>
              <TabsTrigger value="imagine" className="flex-1 sm:flex-none">{t.function.imagine}</TabsTrigger>
              <TabsTrigger value="video" className="flex-1 sm:flex-none">{t.function.video}</TabsTrigger>
            </TabsList>
            <div className="flex items-center gap-2 w-full sm:w-auto shrink-0">
              <Key className="h-4 w-4 text-muted hidden sm:block" />
              <Input
                type="text"
                value={apiKeyValue}
                onChange={(e) => setApiKeyValue(e.target.value)}
                placeholder={t.function.enterApiKey}
                aria-label={t.function.apiKey}
                className="flex-1 sm:w-64"
              />
              <Button size="sm" onClick={handleApply}>{t.function.apply}</Button>
            </div>
          </div>

          <div className="flex-1 flex flex-col min-h-0 relative">
            <TabsContent value="chat" keepMounted className="absolute inset-0 m-0 data-[state=inactive]:hidden flex flex-col">
              <ChatPanel />
            </TabsContent>

            <TabsContent value="imagine" className="absolute inset-0 m-0 data-[state=inactive]:hidden overflow-auto">
              <ImaginePanel />
            </TabsContent>

            <TabsContent value="video" className="absolute inset-0 m-0 data-[state=inactive]:hidden overflow-auto">
              <VideoPanel />
            </TabsContent>
          </div>
        </Tabs>
      </div>
    </ConfirmProvider>
  )
}

function PanelSkeleton() {
  return <div className="h-full animate-pulse rounded-lg bg-[rgba(0,0,0,0.04)]" />
}
