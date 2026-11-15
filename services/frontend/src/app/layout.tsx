import type { Metadata, Viewport } from 'next'
import '@/styles/globals.css'
import { Providers } from './providers'

export const metadata: Metadata = {
    title: 'Pociąg do Predykcji',
    description: 'Monitoring i predykcja ruchu kolejowego w Polsce',
}

// viewport-fit=cover lets the shell pad around notches and the home indicator (env(safe-area-inset-*)).
export const viewport: Viewport = {
    width: 'device-width',
    initialScale: 1,
    viewportFit: 'cover',
    themeColor: '#1a1d27',
    colorScheme: 'dark',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
    return (
        <html lang="pl" className="dark">
            <body>
                <Providers>{children}</Providers>
            </body>
        </html>
    )
}
