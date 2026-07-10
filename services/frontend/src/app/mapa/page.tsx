'use client'
import dynamic from 'next/dynamic'
import { NavShell } from '@/components/NavShell'
import { Spinner } from '@/lib/ui'

const TrafficMapClient = dynamic(
    () => import('@/components/TrafficMapClient').then((m) => m.TrafficMapClient),
    {
        ssr: false,
        loading: () => (
            <div className="flex items-center justify-center w-full h-full">
                <Spinner className="h-8 w-8" />
            </div>
        ),
    },
)

export default function MapaPage() {
    return (
        <NavShell title="Mapa sieci">
            <div style={{ height: 'calc(100vh - 57px)' }} className="md:h-screen relative">
                <TrafficMapClient />
            </div>
        </NavShell>
    )
}
