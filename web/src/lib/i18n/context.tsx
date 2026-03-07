'use client'

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { defaultDictionary, loadDictionary, type Language, type Dictionary } from './dictionaries'

interface LanguageContextProps {
    language: Language
    setLanguage: (lang: Language) => void
    t: Dictionary
}

const LanguageContext = createContext<LanguageContextProps | undefined>(undefined)

function buildLocaleCookie(language: Language): string {
    const parts = [`NEXT_LOCALE=${language}`, 'path=/', 'max-age=31536000', 'SameSite=Lax']
    if (window.location.protocol === 'https:') {
        parts.push('Secure')
    }
    return parts.join('; ')
}

export function LanguageProvider({
    children,
    initialLanguage = 'zh',
}: {
    children: ReactNode
    initialLanguage?: Language
}) {
    const [language, setLanguageState] = useState<Language>(initialLanguage)
    const [dictionary, setDictionary] = useState<Dictionary>(defaultDictionary)

    useEffect(() => {
        const match = document.cookie.match(new RegExp('(^| )NEXT_LOCALE=([^;]+)'))
        if (match && (match[2] === 'zh' || match[2] === 'en')) {
            setLanguageState(match[2] as Language)
        } else {
            document.cookie = buildLocaleCookie(initialLanguage)
        }
    }, [initialLanguage])

    const setLanguage = (lang: Language) => {
        setLanguageState(lang)
        document.cookie = buildLocaleCookie(lang)
    }

    useEffect(() => {
        document.documentElement.lang = language
        let cancelled = false

        void loadDictionary(language).then((nextDictionary) => {
            if (!cancelled) {
                setDictionary(nextDictionary)
            }
        })

        return () => {
            cancelled = true
        }
    }, [language])

    return (
        <LanguageContext.Provider value={{ language, setLanguage, t: dictionary }}>
            {children}
        </LanguageContext.Provider>
    )
}

export function useTranslation() {
    const context = useContext(LanguageContext)
    if (context === undefined) {
        throw new Error('useTranslation must be used within a LanguageProvider')
    }
    return context
}
