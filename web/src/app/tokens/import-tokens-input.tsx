import { Label, Textarea } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'

interface ImportTokensInputProps {
  tokens: string
  onChange: (value: string) => void
}

export function ImportTokensInput({ tokens, onChange }: ImportTokensInputProps) {
  const { t } = useTranslation()

  const lines = tokens.split('\n').map(l => l.trim()).filter(l => l.length > 0)
  const validLines = lines.filter(l => l.length >= 20)
  const invalidCount = lines.length - validLines.length

  return (
    <div className="space-y-2">
      <Label htmlFor="tokens">{t.tokens.ssoLabel}</Label>
      <Textarea
        id="tokens"
        value={tokens}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t.tokens.pasteTokens}
        className="min-h-[150px] font-mono text-sm"
      />
      <p className="text-sm text-muted">
        {`${lines.length} ${t.tokens.lines}, ${validLines.length} ${t.tokens.valid} (${invalidCount} ${t.tokens.tooShort})`}
      </p>
    </div>
  )
}
