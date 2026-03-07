'use client'

import * as React from 'react'
import { Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle, DialogDescription } from './dialog'
import { Button } from './button'
import { useTranslation } from '@/lib/i18n/context'

interface ConfirmOptions {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  variant?: 'default' | 'destructive'
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>

const ConfirmContext = React.createContext<ConfirmFn | null>(null)

export function useConfirm(): ConfirmFn {
  const fn = React.useContext(ConfirmContext)
  if (!fn) throw new Error('useConfirm must be used within ConfirmProvider')
  return fn
}

export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation()
  const [state, setState] = React.useState<{
    open: boolean
    opts: ConfirmOptions
    resolve: ((v: boolean) => void) | null
  }>({ open: false, opts: { title: '' }, resolve: null })

  const confirm = React.useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      setState({ open: true, opts, resolve })
    })
  }, [])

  const handleClose = (result: boolean) => {
    state.resolve?.(result)
    setState({ open: false, opts: { title: '' }, resolve: null })
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Dialog open={state.open} onOpenChange={(open) => { if (!open) handleClose(false) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{state.opts.title}</DialogTitle>
            {state.opts.description && (
              <DialogDescription>{state.opts.description}</DialogDescription>
            )}
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleClose(false)}>
              {state.opts.cancelText || t.common.cancel}
            </Button>
            <Button
              variant={state.opts.variant === 'destructive' ? 'destructive' : 'default'}
              onClick={() => handleClose(true)}
            >
              {state.opts.confirmText || t.common.confirm}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </ConfirmContext.Provider>
  )
}
