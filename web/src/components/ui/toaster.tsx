'use client'

import * as React from 'react'
import { Toast, ToastTitle, ToastDescription } from './toast'

interface ToastData {
  id: string
  title?: string
  description?: string
  variant?: 'default' | 'destructive'
}

interface ToasterContextValue {
  toast: (data: Omit<ToastData, 'id'>) => void
}

const ToasterContext = React.createContext<ToasterContextValue | undefined>(undefined)

export function useToast() {
  const context = React.useContext(ToasterContext)
  if (!context) throw new Error('useToast must be used within ToasterProvider')
  return context
}

export function ToasterProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = React.useState<ToastData[]>([])

  const toast = React.useCallback((data: Omit<ToastData, 'id'>) => {
    const id = Math.random().toString(36).slice(2)
    setToasts((prev) => [...prev, { ...data, id }])
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id))
    }, 5000)
  }, [])

  const removeToast = React.useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  return (
    <ToasterContext.Provider value={{ toast }}>
      {children}
      <div className="fixed bottom-0 right-0 z-[100] flex max-h-screen w-full flex-col-reverse gap-2 p-4 sm:max-w-[420px]">
        {toasts.map((t) => (
          <Toast key={t.id} variant={t.variant} onClose={() => removeToast(t.id)}>
            {t.title && <ToastTitle>{t.title}</ToastTitle>}
            {t.description && <ToastDescription>{t.description}</ToastDescription>}
          </Toast>
        ))}
      </div>
    </ToasterContext.Provider>
  )
}

export function Toaster() {
  return null // ToasterProvider handles rendering
}
