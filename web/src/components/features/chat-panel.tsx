'use client'

import * as React from 'react'
import { Button, Card, CardContent, Dialog, DialogContent, DialogHeader, DialogTitle, Select, SelectOption } from '@/components/ui'
import { getApiKey, type ChatMessage } from '@/lib/function-api'
import { useChatModels } from '@/lib/hooks'
import { RotateCcw, Send, Square, PanelLeftClose, PanelLeft, Plus, Sparkles } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { useConversations } from '@/lib/chat-store'
import { ConversationList } from './conversation-list'
import { AssistantBubble, bubbleCls } from './assistant-bubble'
import { useChatSession } from './use-chat-session'

export function ChatPanel() {
  const { t } = useTranslation()
  const { models: chatModels, isLoading: modelsLoading } = useChatModels()
  const {
    conversations, activeId, activeConversation,
    createConversation, switchConversation, deleteConversation,
    updateMessages, updateModel,
  } = useConversations()

  const [input, setInput] = React.useState('')
  const [selectedModel, setSelectedModel] = React.useState('')
  const [sidebarOpen, setSidebarOpen] = React.useState(true)
  const [mobileSidebarOpen, setMobileSidebarOpen] = React.useState(false)
  const scrollContainerRef = React.useRef<HTMLDivElement>(null)
  const messagesEndRef = React.useRef<HTMLDivElement>(null)
  const textareaRef = React.useRef<HTMLTextAreaElement>(null)
  const lastStreamScrollRef = React.useRef(0)

  const messages = React.useMemo(() => activeConversation?.messages ?? [], [activeConversation?.messages])

  const {
    isStreaming, streamingContent, error, setError,
    streamStats, streamTimerRef, doSend, abort, resetState,
  } = useChatSession({ selectedModel, updateMessages })

  React.useEffect(() => {
    if (chatModels.length > 0 && !selectedModel) setSelectedModel(chatModels[0])
  }, [chatModels, selectedModel])

  // Sync model from active conversation when switching
  React.useEffect(() => {
    if (activeConversation?.model && chatModels.includes(activeConversation.model)) {
      setSelectedModel(activeConversation.model)
    }
  }, [activeId, activeConversation?.model, chatModels])

  React.useEffect(() => {
    const anchor = messagesEndRef.current
    const container = scrollContainerRef.current
    if (!anchor || !container) return

    const isUserScrolledUp = container.scrollHeight - container.scrollTop - container.clientHeight > 100

    if (isStreaming) {
      const now = performance.now()
      if (now - lastStreamScrollRef.current < 120) return
      // Prevent scroll yanking if the user is actively reading history
      if (isUserScrolledUp) return

      lastStreamScrollRef.current = now
      anchor.scrollIntoView({ behavior: 'auto' })
      return
    }

    // Only auto-scroll on complete if the user hasn't heavily scrolled up
    if (!isUserScrolledUp) {
      anchor.scrollIntoView({ behavior: 'smooth' })
    }
  }, [activeId, isStreaming, messages.length, streamingContent])

  const resizeTextarea = () => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 144) + 'px'
  }

  const handleModelChange = (model: string) => {
    setSelectedModel(model)
    if (activeId) updateModel(activeId, model)
  }

  const handleNewSession = () => {
    resetState()
    setInput('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
    createConversation(selectedModel)
    setMobileSidebarOpen(false)
  }

  const handleSwitchConversation = (id: string) => {
    if (isStreaming) abort()
    setError(null)
    switchConversation(id)
    setMobileSidebarOpen(false)
  }

  const handleSend = async () => {
    const trimmed = input.trim()
    if (!trimmed) return

    // Preload Markdown parser to avoid FOUC when response completes
    import('./markdown-renderer').catch(() => {})

    const userMsg: ChatMessage = { id: crypto.randomUUID(), role: 'user', content: trimmed }
    setInput('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'

    let convId = activeId
    if (!convId) {
      convId = createConversation(selectedModel)
    }
    await doSend([...messages, userMsg], convId)
  }

  const handleRetry = (index: number) => {
    if (isStreaming || !activeId) return
    const truncated = messages.slice(0, index + 1)
    doSend(truncated, activeId)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() }
  }

  const hasHistory = messages.length > 0 || streamingContent.length > 0 || isStreaming

  return (
    <Card className="flex-1 flex flex-col min-h-0 border-0 shadow-none md:border md:shadow-sm">
      <CardContent className="p-0 flex flex-1 min-h-0">
        {/* Sidebar */}
        <div className={`${sidebarOpen ? 'w-56' : 'w-0'} shrink-0 border-r transition-all overflow-hidden hidden md:flex flex-col`}>
          <ConversationList
            conversations={conversations}
            activeId={activeId}
            onSelect={handleSwitchConversation}
            onDelete={deleteConversation}
            onCreate={handleNewSession}
          />
        </div>
        {/* Main chat area */}
        <div className="flex-1 flex flex-col min-w-0 min-h-0">
          {/* Toolbar */}
          <div className="flex items-center gap-3 p-4 border-b">
            <button
              type="button"
              onClick={() => setSidebarOpen(!sidebarOpen)}
              className="hidden md:inline-flex p-1 rounded hover:bg-[rgba(0,0,0,0.04)] text-muted hover:text-foreground transition-colors"
              aria-label={t.function.conversations}
            >
              {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeft className="h-4 w-4" />}
            </button>
            <button
              type="button"
              onClick={() => setMobileSidebarOpen(true)}
              className="inline-flex md:hidden p-1 rounded hover:bg-[rgba(0,0,0,0.04)] text-muted hover:text-foreground transition-colors"
              aria-label={t.function.conversations}
            >
              <PanelLeft className="h-4 w-4" />
            </button>
            <div className="flex-1">
              <Select value={selectedModel} onChange={(e) => handleModelChange(e.target.value)} disabled={modelsLoading || chatModels.length === 0}>
                {modelsLoading ? (
                  <SelectOption value="">{t.function.loadingModels}</SelectOption>
                ) : chatModels.length === 0 ? (
                  <SelectOption value="">{t.function.noModelsAvailable}</SelectOption>
                ) : chatModels.map((m) => <SelectOption key={m} value={m}>{m}</SelectOption>)}
              </Select>
            </div>
            <Button size="sm" variant="outline" onClick={handleNewSession} disabled={!hasHistory && !!activeId}>
              <Plus className="h-4 w-4 mr-1" />
              {t.function.newSession}
            </Button>
          </div>
          {/* Messages */}
          <div ref={scrollContainerRef} className="flex-1 overflow-y-auto p-4 space-y-3 relative">
            {messages.length === 0 && !isStreaming && !error && (
              <div className="absolute inset-0 flex flex-col items-center justify-center bg-background/50 backdrop-blur-sm z-10">
                <div className="max-w-md p-6 bg-surface border shadow-lg rounded-xl text-center space-y-4">
                  <div className="mx-auto w-12 h-12 rounded-full bg-primary/8 flex items-center justify-center mb-4">
                    <Sparkles className="h-6 w-6 text-primary" />
                  </div>
                  <h3 className="text-lg font-semibold">{t.function.welcomeTitle}</h3>
                  <p className="text-sm text-muted">
                    {t.function.welcomeDescription}
                  </p>
                  {(!getApiKey() || chatModels.length === 0) && (
                    <div className="pt-4 flex flex-col gap-2">
                      {!getApiKey() && (
                        <div className="text-xs text-amber-600 bg-amber-50 p-2 rounded border border-amber-200">
                          {t.function.configureApiKey}
                        </div>
                      )}
                      {getApiKey() && chatModels.length === 0 && !modelsLoading && (
                        <div className="text-xs text-amber-600 bg-amber-50 p-2 rounded border border-amber-200">
                          {t.function.noModelsHint}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            )}
            {messages.map((msg, i) => (
              <div key={msg.id || i} className={`flex items-start gap-1 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                {msg.role === 'user' && !isStreaming && (
                  <button
                    type="button"
                    onClick={() => handleRetry(i)}
                    className="mt-1.5 p-1 rounded hover:bg-[rgba(0,0,0,0.04)] text-muted hover:text-foreground transition-colors shrink-0"
                    title={t.function.retry}
                    aria-label={t.function.retry}
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                  </button>
                )}
                {msg.role === 'user'
                  ? <div className={bubbleCls(true)}>{msg.content}</div>
                  : <AssistantBubble
                      content={msg.content}
                      stats={!isStreaming && i === messages.length - 1 ? streamStats ?? undefined : undefined}
                    />
                }
              </div>
            ))}
            {isStreaming && (
              <div className="flex justify-start">
                <AssistantBubble
                  content={streamingContent}
                  streaming
                  startedAt={streamTimerRef.current.startedAt}
                />
              </div>
            )}
            {error && <div className="bg-destructive/8 text-destructive text-sm px-4 py-2 rounded-lg">{error}</div>}
            <div ref={messagesEndRef} />
          </div>
          {/* Input */}
          <div className="flex items-end gap-2 p-4 border-t">
            <textarea ref={textareaRef} value={input}
              onChange={(e) => { setInput(e.target.value); resizeTextarea() }}
              onKeyDown={handleKeyDown} placeholder={t.function.typeMessage} rows={1}
              className="flex-1 min-w-0 resize-none rounded-md border px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
              style={{ maxHeight: 144 }} />
            {isStreaming ? (
              <Button size="icon" variant="destructive" onClick={() => abort()} aria-label={t.function.stop}>
                <Square className="h-4 w-4" />
              </Button>
            ) : (
              <Button size="icon" onClick={handleSend} disabled={!input.trim()} aria-label={t.function.send}>
                <Send className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      </CardContent>

      <Dialog open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
        <DialogContent className="h-[70vh] max-w-sm overflow-hidden p-0">
          <DialogHeader className="px-4 pt-4">
            <DialogTitle>{t.function.conversations}</DialogTitle>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-hidden">
            <ConversationList
              conversations={conversations}
              activeId={activeId}
              onSelect={handleSwitchConversation}
              onDelete={deleteConversation}
              onCreate={handleNewSession}
            />
          </div>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
