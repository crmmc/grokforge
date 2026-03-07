import { AdminRouteLayout } from '@/components/layout/admin-route-layout'

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminRouteLayout>{children}</AdminRouteLayout>
}
