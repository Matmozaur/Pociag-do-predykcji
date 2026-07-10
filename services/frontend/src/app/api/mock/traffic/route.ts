import { NextResponse } from 'next/server'
import { trafficGeoJSON } from '@/lib/api'

export const dynamic = 'force-static'

export function GET() {
    return NextResponse.json(trafficGeoJSON)
}
