'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import { X } from 'lucide-react'

interface DialogContextValue {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const DialogContext = React.createContext<DialogContextValue | undefined>(undefined)

function useDialog() {
  const context = React.useContext(DialogContext)
  if (!context) throw new Error('useDialog must be used within Dialog')
  return context
}

interface DialogProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  children: React.ReactNode
}

function Dialog({ open: controlledOpen, onOpenChange, children }: DialogProps) {
  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(false)
  const open = controlledOpen ?? uncontrolledOpen
  const handleOpenChange = onOpenChange ?? setUncontrolledOpen

  return (
    <DialogContext.Provider value={{ open, onOpenChange: handleOpenChange }}>
      {children}
    </DialogContext.Provider>
  )
}

function DialogPortal({ children }: { children: React.ReactNode }) {
  const { open } = useDialog()
  if (!open) return null
  return <>{children}</>
}

function DialogOverlay({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const { onOpenChange } = useDialog()
  return (
    <div
      className={cn('fixed inset-0 z-50 bg-black/40 backdrop-blur-[2px]', className)}
      onClick={() => onOpenChange(false)}
      {...props}
    />
  )
}

function getFocusableElements(container: HTMLElement | null): HTMLElement[] {
  if (!container) return []
  return Array.from(
    container.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )
  )
}

const DialogContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, children, ...props }, ref) => {
    const { t } = useTranslation()
    const { open, onOpenChange } = useDialog()
    const contentRef = React.useRef<HTMLDivElement>(null)
    const previousFocusRef = React.useRef<HTMLElement | null>(null)

    React.useEffect(() => {
      if (!open) return

      previousFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
      const previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'

      const focusable = getFocusableElements(contentRef.current)
      ;(focusable[0] ?? contentRef.current)?.focus()

      return () => {
        document.body.style.overflow = previousOverflow
        previousFocusRef.current?.focus()
      }
    }, [open])

    return (
      <DialogPortal>
        <DialogOverlay />
        <div
          ref={(node) => {
            contentRef.current = node
            if (typeof ref === 'function') {
              ref(node)
            } else if (ref) {
              ref.current = node
            }
          }}
          role="dialog"
          aria-modal="true"
          tabIndex={-1}
          className={cn(
            'fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg translate-x-[-50%] translate-y-[-50%] gap-4 p-6 shadow-[0_8px_32px_rgba(0,0,0,0.12)] rounded-[8px] bg-[rgba(255,255,255,0.95)] backdrop-blur-[40px] saturate-150 border border-[rgba(0,0,0,0.08)]',
            className
          )}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              e.stopPropagation()
              onOpenChange(false)
              return
            }

            if (e.key === 'Tab') {
              const focusable = getFocusableElements(contentRef.current)
              if (focusable.length === 0) {
                e.preventDefault()
                return
              }

              const first = focusable[0]
              const last = focusable[focusable.length - 1]
              const active = document.activeElement

              if (e.shiftKey && active === first) {
                e.preventDefault()
                last.focus()
              } else if (!e.shiftKey && active === last) {
                e.preventDefault()
                first.focus()
              }
            }
          }}
          {...props}
        >
          {children}
          <button
            type="button"
            className="absolute right-3 top-3 w-7 h-7 rounded-[4px] flex items-center justify-center text-muted hover:bg-[rgba(0,0,0,0.05)] hover:text-foreground transition-all"
            onClick={() => onOpenChange(false)}
            aria-label={t.common.close}
          >
            <X className="h-4 w-4" />
            <span className="sr-only">{t.common.close}</span>
          </button>
        </div>
      </DialogPortal>
    )
  }
)
DialogContent.displayName = 'DialogContent'

function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex flex-col space-y-1.5', className)} {...props} />
}

function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2', className)} {...props} />
}

function DialogTitle({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h2 className={cn('text-base font-semibold leading-none tracking-tight text-foreground', className)} {...props} />
}

function DialogDescription({ className, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn('text-[13px] text-muted', className)} {...props} />
}

export { Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle, DialogDescription }
