'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { AlertTriangle, ArrowLeft, Map, Search, Train } from 'lucide-react'
import { cn } from '@/lib/ui'

const NAV_ITEMS = [
    { href: '/mapa', label: 'Mapa', icon: Map },
    { href: '/wyszukaj', label: 'Wyszukaj', icon: Search },
    { href: '/pociagi', label: 'Pociągi', icon: Train },
    { href: '/utrudnienia', label: 'Utrudnienia', icon: AlertTriangle },
]

interface NavShellProps {
    children: React.ReactNode
    title?: string
    showBack?: boolean
}

export function NavShell({ children, title, showBack }: NavShellProps) {
    const pathname = usePathname()
    const router = useRouter()

    return (
        <div className="flex h-screen overflow-hidden bg-[#0f1117]">
            <nav
                aria-label="Nawigacja boczna"
                className="hidden w-56 flex-shrink-0 flex-col border-r border-[#2d3148] bg-[#1a1d27] py-6 md:flex"
            >
                <div className="mb-8 px-4">
                    <div className="flex items-center gap-2">
                        <Train className="text-blue-500" size={22} />
                        <div>
                            <p className="text-sm font-bold text-white">Pociąg do</p>
                            <p className="-mt-0.5 text-xs font-semibold text-blue-400">Predykcji</p>
                        </div>
                    </div>
                </div>

                <ul className="flex flex-col gap-1 px-2">
                    {NAV_ITEMS.map(({ href, label, icon: Icon }) => {
                        const active = pathname === href || pathname.startsWith(`${href}/`)
                        return (
                            <li key={href}>
                                <Link
                                    href={href}
                                    className={cn(
                                        'flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors',
                                        active
                                            ? 'border border-blue-500/20 bg-blue-600/20 text-blue-400'
                                            : 'text-slate-400 hover:bg-[#222536] hover:text-white',
                                    )}
                                >
                                    <Icon size={18} />
                                    {label}
                                </Link>
                            </li>
                        )
                    })}
                </ul>

                <div className="mt-auto px-4 pt-6">
                    <p className="text-xs text-slate-600">v0.1.0 · beta</p>
                </div>
            </nav>

            <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
                <header className="flex items-center gap-3 border-b border-[#2d3148] bg-[#1a1d27] px-4 py-3 md:hidden">
                    {showBack ? (
                        <button
                            onClick={() => router.back()}
                            className="text-slate-400 hover:text-white"
                            aria-label="Wróć"
                        >
                            <ArrowLeft size={20} />
                        </button>
                    ) : (
                        <Train className="text-blue-500" size={20} />
                    )}
                    <span className="text-sm font-semibold text-white">{title ?? 'Pociąg do Predykcji'}</span>
                </header>

                <main className="flex-1 overflow-auto pb-16 md:pb-0">{children}</main>

                <nav
                    aria-label="Nawigacja główna"
                    className="fixed bottom-0 left-0 right-0 flex border-t border-[#2d3148] bg-[#1a1d27] md:hidden"
                >
                    {NAV_ITEMS.map(({ href, label, icon: Icon }) => {
                        const active = pathname === href || pathname.startsWith(`${href}/`)
                        return (
                            <Link
                                key={href}
                                href={href}
                                className={cn(
                                    'flex flex-1 flex-col items-center gap-1 py-2 text-xs font-medium transition-colors',
                                    active ? 'text-blue-400' : 'text-slate-500 hover:text-slate-300',
                                )}
                            >
                                <Icon size={20} />
                                {label}
                            </Link>
                        )
                    })}
                </nav>
            </div>
        </div>
    )
}
