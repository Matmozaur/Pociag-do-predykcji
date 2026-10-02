'use client'
import { useParams } from 'next/navigation'
import { NavShell } from '@/components/NavShell'
import { StationBoard } from '@/components/StationBoard'

export default function StationBoardPage() {
    const { id } = useParams<{ id: string }>()
    const stationId = id && /^\d+$/.test(id) ? parseInt(id, 10) : null

    return (
        <NavShell title="Tablica stacyjna" showBack>
            <div className="max-w-2xl mx-auto p-4 md:p-6">
                <StationBoard key={stationId} stationId={stationId} variant="page" />
            </div>
        </NavShell>
    )
}
