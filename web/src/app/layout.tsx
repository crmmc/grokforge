import type { Metadata } from 'next'
import './globals.css'
import { LanguageProvider } from '@/lib/i18n/context'

export const metadata: Metadata = {
  title: 'GrokForge Admin',
  description: 'GrokForge Administration Panel',
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="zh">
      <body className="bg-background text-foreground font-sans antialiased min-h-screen relative">
        {/* Fluent 2 mesh gradient background */}
        <div className="fixed inset-0 pointer-events-none -z-10">
          <div className="absolute top-[-20%] left-[-10%] w-[50%] h-[50%] bg-[#005FB8]/5 rounded-full blur-[100px]" />
          <div className="absolute bottom-[-10%] right-[-10%] w-[60%] h-[60%] bg-[#0091FF]/5 rounded-full blur-[120px]" />
          <div className="absolute top-[20%] right-[10%] w-[40%] h-[40%] bg-[#C42B1C]/3 rounded-full blur-[90px]" />
        </div>

        <LanguageProvider>
          {children}
        </LanguageProvider>
      </body>
    </html>
  )
}
