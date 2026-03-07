import { zh } from './zh'

export type Language = 'zh' | 'en'
export type Dictionary = typeof zh

export const defaultDictionary = zh

export async function loadDictionary(language: Language): Promise<Dictionary> {
  if (language === 'en') {
    const mod = await import('./en')
    return mod.en
  }

  return zh
}
