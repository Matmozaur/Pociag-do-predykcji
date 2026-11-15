'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { AlertTriangle, ArrowLeft, Map, MapPin, Search, Train } from 'lucide-react'
import { cn } from '@/lib/ui'

const NAV_ITEMS = [
    { href: '/mapa', label: 'Mapa', icon: Map },
    { href: '/wyszukaj', label: 'Wyszukaj', icon: Search },
    { href: '/stacje', label: 'Stacje', icon: MapPin },
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
        <div className="flex h-dvh overflow-hidden bg-[#0f1117]">
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
                <header className="flex h-[calc(3.25rem+env(safe-area-inset-top))] flex-none items-center gap-2 border-b border-[#2d3148] bg-[#1a1d27] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))] pt-[env(safe-area-inset-top)] md:hidden">
                    {showBack ? (
                        <button
                            type="button"
                            // A deep link has no in-app history to go back to; fall back to the map.
                            onClick={() => (window.history.length > 1 ? router.back() : router.push('/mapa'))}
                            className="flex h-10 w-10 flex-none items-center justify-center rounded-lg text-slate-400 transition-colors hover:bg-[#222536] hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
                            aria-label="Wróć"
                        >
                            <ArrowLeft size={20} />
                        </button>
                    ) : (
                        <span className="flex h-10 w-10 flex-none items-center justify-center" aria-hidden="true">
                            <Train className="text-blue-500" size={20} />
                        </span>
                    )}
                    <span className="min-w-0 truncate text-sm font-semibold text-white">{title ?? 'Pociąg do Predykcji'}</span>
                </header>

                {/* Bottom padding keeps the last content clear of the fixed tab bar and the home indicator. */}
                <main className="min-h-0 flex-1 overflow-auto overscroll-contain pb-[calc(3.5rem+env(safe-area-inset-bottom))] md:pb-0">{children}</main>

                {/* z above Leaflet panes and controls (up to 1000) and the map search bar (1001). */}
                <nav
                    aria-label="Nawigacja główna"
                    className="fixed bottom-0 left-0 right-0 z-[1100] flex border-t border-[#2d3148] bg-[#1a1d27] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] pr-[env(safe-area-inset-right)] md:hidden"
                >
                    {NAV_ITEMS.map(({ href, label, icon: Icon }) => {
                        const active = pathname === href || pathname.startsWith(`${href}/`)
                        return (
                            <Link
                                key={href}
                                href={href}
                                aria-current={active ? 'page' : undefined}
                                className={cn(
                                    'flex h-14 min-w-0 flex-1 flex-col items-center justify-center gap-1 text-[10px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-500 min-[360px]:text-[11px] min-[400px]:text-xs',
                                    active ? 'text-blue-400' : 'text-slate-500 hover:text-slate-300',
                                )}
                            >
                                <span
                                    className={cn(
                                        'flex h-7 w-12 items-center justify-center rounded-full transition-colors',
                                        active && 'bg-blue-600/20',
                                    )}
                                    aria-hidden="true"
                                >
                                    <Icon size={20} />
                                </span>
                                <span className="max-w-full truncate px-0.5">{label}</span>
                            </Link>
                        )
                    })}
                </nav>
            </div>
        </div>
    )
}
