'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ChatMessage } from './function-api'

export interface Conversation {
  id: string
  title: string
  messages: ChatMessage[]
  model: string
  createdAt: number
  updatedAt: number
}

const STORAGE_KEY = 'grokforge_conversations'
const DEFAULT_TITLE = 'New conversation'

function loadConversations(): Conversation[] {
  if (typeof window === 'undefined') return []
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

function saveConversations(convs: Conversation[]): void {
  if (typeof window === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(convs))
  } catch { /* quota exceeded — silently fail */ }
}

function autoTitle(messages: ChatMessage[]): string | null {
  const first = messages.find((m) => m.role === 'user')
  if (!first) return null
  return first.content.length > 30 ? first.content.slice(0, 30) + '...' : first.content
}

export function useConversations() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)

  // Load from localStorage on mount
  useEffect(() => {
    const loaded = loadConversations()
    setConversations(loaded)
    if (loaded.length > 0) {
      const most = loaded.reduce((a, b) => (a.updatedAt > b.updatedAt ? a : b))
      setActiveId(most.id)
    }
  }, [])

  const activeConversation = conversations.find((c) => c.id === activeId) ?? null

  const createConversation = useCallback((model: string): string => {
    const id = Date.now().toString(36) + Math.random().toString(36).slice(2, 6)
    const conv: Conversation = {
      id,
      title: DEFAULT_TITLE,
      messages: [],
      model,
      createdAt: Date.now(),
      updatedAt: Date.now(),
    }
    setConversations((prev) => {
      const next = [conv, ...prev]
      saveConversations(next)
      return next
    })
    setActiveId(id)
    return id
  }, [])

  const switchConversation = useCallback((id: string) => {
    setActiveId(id)
  }, [])

  const deleteConversation = useCallback((id: string) => {
    setConversations((prev) => {
      const next = prev.filter((c) => c.id !== id)
      saveConversations(next)
      // Update activeId inline to avoid extra localStorage read
      setActiveId((prevId) => {
        if (prevId !== id) return prevId
        if (next.length === 0) return null
        return next.reduce((a, b) => (a.updatedAt > b.updatedAt ? a : b)).id
      })
      return next
    })
  }, [])

  const updateMessages = useCallback((id: string, messages: ChatMessage[]) => {
    setConversations((prev) => {
      const next = prev.map((c) => {
        if (c.id !== id) return c
        const title = c.title === DEFAULT_TITLE ? (autoTitle(messages) ?? c.title) : c.title
        return { ...c, messages, title, updatedAt: Date.now() }
      })
      saveConversations(next)
      return next
    })
  }, [])

  const updateModel = useCallback((id: string, model: string) => {
    setConversations((prev) => {
      const next = prev.map((c) => (c.id !== id ? c : { ...c, model, updatedAt: Date.now() }))
      saveConversations(next)
      return next
    })
  }, [])

  return {
    conversations,
    activeId,
    activeConversation,
    createConversation,
    switchConversation,
    deleteConversation,
    updateMessages,
    updateModel,
  }
}
