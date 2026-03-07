import { ToasterProvider } from '@/components/ui'

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return <ToasterProvider>{children}</ToasterProvider>
}
