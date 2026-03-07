'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

interface TooltipProviderProps {
  children: React.ReactNode
  delayDuration?: number
}

const TooltipDelayContext = React.createContext(200)

function TooltipProvider({ children, delayDuration = 200 }: TooltipProviderProps) {
  return (
    <TooltipDelayContext.Provider value={delayDuration}>
      {children}
    </TooltipDelayContext.Provider>
  )
}

interface TooltipContextValue {
  open: boolean
  setOpen: (open: boolean) => void
  triggerRef: React.RefObject<HTMLElement | null>
}

const TooltipContext = React.createContext<TooltipContextValue | undefined>(undefined)

function useTooltipContext() {
  const ctx = React.useContext(TooltipContext)
  if (!ctx) throw new Error('useTooltipContext must be used within Tooltip')
  return ctx
}

function Tooltip({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false)
  const triggerRef = React.useRef<HTMLElement | null>(null)

  return (
    <TooltipContext.Provider value={{ open, setOpen, triggerRef }}>
      {children}
    </TooltipContext.Provider>
  )
}

interface TooltipTriggerProps extends React.HTMLAttributes<HTMLElement> {
  asChild?: boolean
}

const TooltipTrigger = React.forwardRef<HTMLElement, TooltipTriggerProps>(
  ({ asChild, children, ...props }, ref) => {
    const { setOpen, triggerRef } = useTooltipContext()
    const delay = React.useContext(TooltipDelayContext)
    const timeoutRef = React.useRef<ReturnType<typeof setTimeout>>(undefined)

    const handleEnter = () => {
      timeoutRef.current = setTimeout(() => setOpen(true), delay)
    }
    const handleLeave = () => {
      clearTimeout(timeoutRef.current)
      setOpen(false)
    }

    const setRefs = React.useCallback((node: HTMLElement | null) => {
      triggerRef.current = node
      if (typeof ref === 'function') ref(node)
      else if (ref) (ref as React.MutableRefObject<HTMLElement | null>).current = node
    }, [ref, triggerRef])

    if (asChild && React.isValidElement(children)) {
      return React.cloneElement(children as React.ReactElement<Record<string, unknown>>, {
        ref: setRefs,
        onMouseEnter: handleEnter,
        onMouseLeave: handleLeave,
        onFocus: handleEnter,
        onBlur: handleLeave,
        ...props,
      })
    }

    return (
      <span
        ref={setRefs as React.Ref<HTMLSpanElement>}
        onMouseEnter={handleEnter}
        onMouseLeave={handleLeave}
        onFocus={handleEnter}
        onBlur={handleLeave}
        {...props}
      >
        {children}
      </span>
    )
  }
)
TooltipTrigger.displayName = 'TooltipTrigger'

interface TooltipContentProps extends React.HTMLAttributes<HTMLDivElement> {
  sideOffset?: number
  side?: 'top' | 'bottom'
}

function TooltipContent({ className, children, sideOffset = 6, side = 'top', ...props }: TooltipContentProps) {
  const { open, triggerRef } = useTooltipContext()
  const [pos, setPos] = React.useState({ top: 0, left: 0 })
  const contentRef = React.useRef<HTMLDivElement>(null)

  React.useEffect(() => {
    if (!open || !triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    const contentEl = contentRef.current
    const cw = contentEl?.offsetWidth ?? 0

    const top = side === 'top'
      ? rect.top - sideOffset
      : rect.bottom + sideOffset
    const left = rect.left + rect.width / 2 - cw / 2

    setPos({ top, left: Math.max(4, left) })
  }, [open, sideOffset, side, triggerRef])

  if (!open) return null

  return (
    <div
      ref={contentRef}
      className={cn(
        'fixed z-[100] rounded-[4px] bg-[#2D2D2D] px-2.5 py-1 text-[12px] text-white shadow-[0_4px_12px_rgba(0,0,0,0.15)] pointer-events-none',
        side === 'top' && '-translate-y-full',
        className
      )}
      style={{ top: pos.top, left: pos.left }}
      {...props}
    >
      {children}
    </div>
  )
}

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }
