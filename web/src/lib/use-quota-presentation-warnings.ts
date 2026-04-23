'use client'

import { useEffect } from 'react'

export function useQuotaPresentationWarnings(source: string, messages: string[]) {
  const joinedMessages = messages.join('\n')

  useEffect(() => {
    if (!joinedMessages) {
      return
    }

    console.warn(`[quota-presentation:${source}] ${joinedMessages}`)
  }, [joinedMessages, source])
}
