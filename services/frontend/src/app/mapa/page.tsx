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
            {/* Fills <main> (between the mobile header and tab bar), so the page itself never scrolls. */}
            <div className="relative h-full overflow-hidden">
                <MapHomeClient />
            </div>
        </NavShell>
    )
}
