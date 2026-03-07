'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

interface TabsContextValue {
  value: string
  onValueChange: (value: string) => void
  baseId: string
}

const TabsContext = React.createContext<TabsContextValue | undefined>(undefined)

function useTabs() {
  const context = React.useContext(TabsContext)
  if (!context) throw new Error('useTabs must be used within Tabs')
  return context
}

interface TabsProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: string
  defaultValue?: string
  onValueChange?: (value: string) => void
}

function Tabs({ value: controlledValue, defaultValue, onValueChange, className, children, ...props }: TabsProps) {
  const [uncontrolledValue, setUncontrolledValue] = React.useState(defaultValue ?? '')
  const value = controlledValue ?? uncontrolledValue
  const handleValueChange = onValueChange ?? setUncontrolledValue
  const baseId = React.useId()

  return (
    <TabsContext.Provider value={{ value, onValueChange: handleValueChange, baseId }}>
      <div className={cn('', className)} {...props}>
        {children}
      </div>
    </TabsContext.Provider>
  )
}

const TabsList = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      role="tablist"
      className={cn(
        'inline-flex h-9 items-center gap-1 border-b border-[rgba(0,0,0,0.06)] pb-0',
        className
      )}
      {...props}
    />
  )
)
TabsList.displayName = 'TabsList'

interface TabsTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  value: string
}

const TabsTrigger = React.forwardRef<HTMLButtonElement, TabsTriggerProps>(
  ({ className, value, ...props }, ref) => {
    const { value: selectedValue, onValueChange, baseId } = useTabs()
    const isSelected = selectedValue === value
    const localRef = React.useRef<HTMLButtonElement>(null)

    const setRefs = (node: HTMLButtonElement | null) => {
      localRef.current = node
      if (typeof ref === 'function') {
        ref(node)
      } else if (ref) {
        ref.current = node
      }
    }

    const moveFocus = (direction: 'next' | 'prev' | 'first' | 'last') => {
      const current = localRef.current
      const list = current?.closest('[role="tablist"]')
      if (!current || !list) return

      const tabs = Array.from(list.querySelectorAll<HTMLButtonElement>('[role="tab"]'))
      const currentIndex = tabs.indexOf(current)
      if (currentIndex === -1) return

      let nextIndex = currentIndex
      if (direction === 'next') nextIndex = (currentIndex + 1) % tabs.length
      if (direction === 'prev') nextIndex = (currentIndex - 1 + tabs.length) % tabs.length
      if (direction === 'first') nextIndex = 0
      if (direction === 'last') nextIndex = tabs.length - 1

      const nextTab = tabs[nextIndex]
      nextTab?.focus()
      const nextValue = nextTab?.dataset.value
      if (nextValue) onValueChange(nextValue)
    }

    return (
      <button
        ref={setRefs}
        type="button"
        role="tab"
        id={`${baseId}-trigger-${value}`}
        aria-selected={isSelected}
        aria-controls={`${baseId}-panel-${value}`}
        tabIndex={isSelected ? 0 : -1}
        data-value={value}
        onClick={() => onValueChange(value)}
        onKeyDown={(e) => {
          props.onKeyDown?.(e)
          if (e.defaultPrevented) return
          if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
            e.preventDefault()
            moveFocus('next')
          } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
            e.preventDefault()
            moveFocus('prev')
          } else if (e.key === 'Home') {
            e.preventDefault()
            moveFocus('first')
          } else if (e.key === 'End') {
            e.preventDefault()
            moveFocus('last')
          }
        }}
        className={cn(
          'relative inline-flex items-center justify-center px-3 py-2 text-[13px] font-medium transition-all duration-150 select-none rounded-t-[4px]',
          isSelected
            ? 'text-foreground'
            : 'text-muted hover:text-foreground hover:bg-[rgba(0,0,0,0.03)]',
          className
        )}
        {...props}
      >
        {props.children}
        {isSelected && (
          <div className="absolute bottom-0 left-2 right-2 h-[2px] bg-primary rounded-full" />
        )}
      </button>
    )
  }
)
TabsTrigger.displayName = 'TabsTrigger'

interface TabsContentProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string
  keepMounted?: boolean
}

const TabsContent = React.forwardRef<HTMLDivElement, TabsContentProps>(
  ({ className, value, keepMounted = false, ...props }, ref) => {
    const { value: selectedValue, baseId } = useTabs()
    const isSelected = selectedValue === value
    if (!isSelected && !keepMounted) return null

    return (
      <div
        ref={ref}
        id={`${baseId}-panel-${value}`}
        role="tabpanel"
        aria-labelledby={`${baseId}-trigger-${value}`}
        aria-hidden={!isSelected}
        hidden={!isSelected}
        className={cn('mt-4', className)}
        {...props}
      />
    )
  }
)
TabsContent.displayName = 'TabsContent'

export { Tabs, TabsList, TabsTrigger, TabsContent }
