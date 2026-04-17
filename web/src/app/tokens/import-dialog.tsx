'use client'

import { useState } from 'react'
import { useBatchTokens } from '@/lib/hooks'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  Button, Label, Select, SelectOption, Input, Switch,
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { useTranslation } from '@/lib/i18n/context'
import { ImportTokensInput } from './import-tokens-input'
import { getAPIErrorMessage } from '@/lib/api-client'

interface ImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportDialog({ open, onOpenChange }: ImportDialogProps) {
  const [tokens, setTokens] = useState('')
  const [pool, setPool] = useState<string>('ssoBasic')
  const [chatQuota, setChatQuota] = useState<string>('')
  const [imageQuota, setImageQuota] = useState<string>('')
  const [videoQuota, setVideoQuota] = useState<string>('')
  const [priority, setPriority] = useState<string>('')
  const [importStatus, setImportStatus] = useState<string>('active')
  const [remark, setRemark] = useState('')
  const [nsfwEnabled, setNsfwEnabled] = useState(false)
  const { toast } = useToast()
  const batchTokens = useBatchTokens()
  const { t } = useTranslation()

  const lines = tokens.split('\n').map(l => l.trim()).filter(l => l.length > 0)
  const validLines = lines.filter(l => l.length >= 20)

  const handleImport = async () => {
    if (validLines.length === 0) {
      toast({
        title: t.common.error,
        description: t.tokens.importFailed,
        variant: 'destructive',
      })
      return
    }

    try {
      const chatQuotaNum = parseOptionalInt(chatQuota)
      const imageQuotaNum = parseOptionalInt(imageQuota)
      const videoQuotaNum = parseOptionalInt(videoQuota)
      const priorityNum = parseOptionalInt(priority)
      const result = await batchTokens.mutateAsync({
        operation: 'import',
        tokens: validLines,
        pool,
        chat_quota: chatQuotaNum,
        image_quota: imageQuotaNum,
        video_quota: videoQuotaNum,
        priority: priorityNum,
        status: importStatus !== 'active' ? importStatus : undefined,
        remark: remark || undefined,
        nsfw_enabled: nsfwEnabled,
      })

      toast({
        title: t.common.success,
        description: result.failed > 0
          ? t.tokens.importPartial.replace('{success}', String(result.success)).replace('{failed}', String(result.failed))
          : t.tokens.importSuccess.replace('{count}', String(result.success)),
      })

      if (result.success > 0) {
        setTokens('')
        setPool('ssoBasic')
        setChatQuota('')
        setImageQuota('')
        setVideoQuota('')
        setPriority('')
        setImportStatus('active')
        setRemark('')
        setNsfwEnabled(false)
        onOpenChange(false)
      }
    } catch (error) {
      toast({
        title: t.common.error,
        description: getAPIErrorMessage(error, t.tokens.importFailed),
        variant: 'destructive',
      })
    }
  }

  const handleClose = () => {
    setTokens('')
    setPool('ssoBasic')
    setChatQuota('')
    setImageQuota('')
    setVideoQuota('')
    setPriority('')
    setImportStatus('active')
    setRemark('')
    setNsfwEnabled(false)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>{t.tokens.importTitle}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <ImportTokensInput tokens={tokens} onChange={setTokens} />

          <div className="grid grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label htmlFor="pool">{t.tokens.pool}</Label>
              <Select value={pool} onChange={(e) => setPool(e.target.value)}>
                <SelectOption value="ssoBasic">{t.dashboard.basicPool}</SelectOption>
                <SelectOption value="ssoSuper">{t.dashboard.superPool}</SelectOption>
                <SelectOption value="ssoHeavy">{t.dashboard.heavyPool}</SelectOption>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="chatQuota">{t.tokens.chatQuota}</Label>
              <Input
                id="chatQuota"
                type="number"
                value={chatQuota}
                onChange={(e) => setChatQuota(e.target.value)}
                placeholder="80"
                min={0}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="imageQuota">{t.tokens.imageQuota}</Label>
              <Input
                id="imageQuota"
                type="number"
                value={imageQuota}
                onChange={(e) => setImageQuota(e.target.value)}
                placeholder="20"
                min={0}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="videoQuota">{t.tokens.videoQuota}</Label>
              <Input
                id="videoQuota"
                type="number"
                value={videoQuota}
                onChange={(e) => setVideoQuota(e.target.value)}
                placeholder="5"
                min={0}
              />
            </div>
          </div>
          <div className="grid grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label htmlFor="priority">{t.tokens.priority}</Label>
              <Input
                id="priority"
                type="number"
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                placeholder={t.tokens.priorityPlaceholder}
                min={0}
              />
            </div>
            <div className="space-y-2">
                <Label htmlFor="importStatus">{t.tokens.importStatus}</Label>
                <Select value={importStatus} onChange={(e) => setImportStatus(e.target.value)}>
                  <SelectOption value="active">{t.tokens.importStatusActive}</SelectOption>
                <SelectOption value="disabled">{t.tokens.importStatusDisabled}</SelectOption>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="remark">{t.tokens.remarkOptional}</Label>
            <Input
              id="remark"
              value={remark}
              onChange={(e) => setRemark(e.target.value)}
              placeholder={t.tokens.remarkPlaceholder}
              maxLength={30}
            />
          </div>

          <div className="flex items-center justify-between">
            <Label htmlFor="nsfw_enabled">NSFW {t.tokens.nsfwEnabled}</Label>
            <Switch
              id="nsfw_enabled"
              checked={nsfwEnabled}
              onCheckedChange={(checked) => setNsfwEnabled(checked)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            {t.common.cancel}
          </Button>
          <Button
            onClick={handleImport}
            disabled={batchTokens.isPending || validLines.length === 0}
          >
            {batchTokens.isPending
              ? t.common.importing
              : `${t.common.import} ${validLines.length}`
            }
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function parseOptionalInt(value: string): number | undefined {
  if (!value) return undefined
  const parsed = parseInt(value, 10)
  return Number.isNaN(parsed) ? undefined : parsed
}
