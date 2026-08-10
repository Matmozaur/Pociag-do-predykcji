'use client'
import dynamic from 'next/dynamic'
import { NavShell } from '@/components/NavShell'
import { Spinner } from '@/lib/ui'

const MapHomeClient = dynamic(
    () => import('@/components/MapHomeClient').then((m) => m.MapHomeClient),
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
            <div style={{ height: 'calc(100vh - 57px)' }} className="relative overflow-hidden md:h-screen">
                <MapHomeClient />
            </div>
        </NavShell>
    )
}
