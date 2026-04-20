'use client'

import dynamic from 'next/dynamic'
import { Settings, Loader2, AlertCircle } from 'lucide-react'
import { useConfig, useUpdateConfig } from '@/lib/hooks'
import { Alert, AlertDescription, Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui'
import { useToast } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import type { ConfigResponse } from '@/types'

const GeneralConfigForm = dynamic(
  () => import('./general-config-form').then((mod) => mod.GeneralConfigForm),
  { loading: () => <ConfigFormSkeleton /> }
)

const ModelsConfigForm = dynamic(
  () => import('./models-config-form').then((mod) => mod.ModelsConfigForm),
  { loading: () => <ConfigFormSkeleton /> }
)

const ModelCatalogTable = dynamic(
  () => import('./model-catalog-table').then((mod) => mod.ModelCatalogTable),
  { loading: () => <ConfigFormSkeleton /> }
)

export default function SettingsPage() {
  const { data: config, isLoading, error } = useConfig()
  const updateConfig = useUpdateConfig()
  const { toast } = useToast()
  const { t } = useTranslation()

  const handleSubmit = (data: Partial<ConfigResponse>) => {
    updateConfig.mutate(data, {
      onSuccess: () => {
        toast({
          title: t.common.success,
          description: t.config.saveSuccess,
        })
      },
      onError: (err) => {
        toast({
          title: t.common.error,
          description: `${t.config.saveError}: ${err.message}`,
          variant: 'destructive',
        })
      },
    })
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-muted" />
      </div>
    )
  }

  if (error || !config) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          {t.common.loadFailed}{': '}{error?.message || t.common.unknownError}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-8 max-w-4xl">
      <div className="flex items-start gap-4">
        <div className="rounded-lg bg-secondary p-2 mt-1 drop-shadow-sm">
          <Settings className="h-6 w-6 text-foreground" />
        </div>
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-bold tracking-tight">{t.settings.title}</h1>
          <p className="text-muted text-sm">
            {t.settings.description}
          </p>
        </div>
      </div>

      <Tabs defaultValue="general" className="w-full">
        <TabsList className="grid w-full max-w-lg grid-cols-3 h-11 mb-6">
          <TabsTrigger value="general" className="text-base">{t.settings.generalTab}</TabsTrigger>
          <TabsTrigger value="models" className="text-base">{t.settings.modelsTab}</TabsTrigger>
          <TabsTrigger value="catalog" className="text-base">{t.settings.catalogTab}</TabsTrigger>
        </TabsList>
        <div className="flex-1 min-w-0">
          <TabsContent value="general" className="m-0 focus-visible:outline-none">
            <GeneralConfigForm
              config={config}
              onSubmit={handleSubmit}
              isPending={updateConfig.isPending}
            />
          </TabsContent>
          <TabsContent value="models" className="m-0 focus-visible:outline-none">
            <ModelsConfigForm
              config={config}
              onSubmit={handleSubmit}
              isPending={updateConfig.isPending}
            />
          </TabsContent>
          <TabsContent value="catalog" className="m-0 focus-visible:outline-none">
            <ModelCatalogTable />
          </TabsContent>
        </div>
      </Tabs>
    </div>
  )
}

function ConfigFormSkeleton() {
  return <div className="h-96 animate-pulse rounded-lg bg-[rgba(0,0,0,0.04)]" />
}
