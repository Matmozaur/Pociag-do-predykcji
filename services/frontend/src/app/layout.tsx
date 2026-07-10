import type { Metadata } from 'next'
import '@/styles/globals.css'
import { Providers } from './providers'

export const metadata: Metadata = {
    title: 'Pociąg do Predykcji',
    description: 'Monitoring i predykcja ruchu kolejowego w Polsce',
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
