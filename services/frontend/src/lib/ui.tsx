import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type {
    ButtonHTMLAttributes,
    InputHTMLAttributes,
    KeyboardEvent,
    ReactNode,
} from 'react'

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs))
}

type BadgeVariant = 'default' | 'success' | 'warning' | 'danger' | 'outline'

interface BadgeProps {
    children: ReactNode
    variant?: BadgeVariant
    className?: string
}

export function Badge({ children, variant = 'default', className }: BadgeProps) {
    return (
        <span
            className={cn(
                'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                variant === 'success' && 'bg-green-500/20 text-green-400',
                variant === 'warning' && 'bg-amber-500/20 text-amber-400',
                variant === 'danger' && 'bg-red-500/20 text-red-400',
                variant === 'outline' && 'border border-slate-600 text-slate-300',
                variant === 'default' && 'bg-blue-500/20 text-blue-400',
                className,
            )}
        >
            {children}
        </span>
    )
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: 'primary' | 'secondary' | 'ghost'
    size?: 'sm' | 'md' | 'lg'
}

export function Button({
    children,
    variant = 'primary',
    size = 'md',
    className,
    ...props
}: ButtonProps) {
    return (
        <button
            className={cn(
                'inline-flex items-center justify-center rounded-lg font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:pointer-events-none disabled:opacity-50',
                variant === 'primary' && 'bg-blue-600 text-white hover:bg-blue-700',
                variant === 'secondary' && 'border border-[#2d3148] bg-[#222536] text-slate-200 hover:bg-[#2d3148]',
                variant === 'ghost' && 'text-slate-400 hover:bg-[#222536] hover:text-white',
                size === 'sm' && 'px-3 py-1.5 text-sm',
                size === 'md' && 'px-4 py-2 text-sm',
                size === 'lg' && 'px-6 py-3 text-base',
                className,
            )}
            {...props}
        >
            {children}
        </button>
    )
}

interface CardProps {
    children: ReactNode
    className?: string
    onClick?: () => void
}

export function Card({ children, className, onClick }: CardProps) {
    if (onClick) {
        return (
            <div
                role="button"
                tabIndex={0}
                className={cn(
                    'rounded-xl border border-[#2d3148] bg-[#1a1d27] p-4',
                    'cursor-pointer transition-colors hover:border-blue-500/50 hover:bg-[#1e2133]',
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500',
                    className,
                )}
                onClick={onClick}
                onKeyDown={(e: KeyboardEvent<HTMLDivElement>) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        onClick()
                    }
                }}
            >
                {children}
            </div>
        )
    }

    return <div className={cn('rounded-xl border border-[#2d3148] bg-[#1a1d27] p-4', className)}>{children}</div>
}

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
    label?: string
    error?: string
}

export function Input({ label, error, className, ...props }: InputProps) {
    return (
        <div className="flex flex-col gap-1">
            {label && <label className="text-xs font-medium uppercase tracking-wide text-slate-400">{label}</label>}
            <input
                className={cn(
                    'w-full rounded-lg border border-[#2d3148] bg-[#222536] px-3 py-2 text-slate-100 placeholder-slate-500',
                    'transition-colors focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500',
                    error && 'border-red-500',
                    className,
                )}
                {...props}
            />
            {error && <p className="text-xs text-red-400">{error}</p>}
        </div>
    )
}

export function Spinner({ className }: { className?: string }) {
    return (
        <div
            role="status"
            aria-label="Ładowanie..."
            className={cn('h-5 w-5 animate-spin rounded-full border-2 border-slate-600 border-t-blue-500', className)}
        >
            <span className="sr-only">Ładowanie...</span>
        </div>
    )
}

interface EmptyStateProps {
    icon?: ReactNode
    title: string
    description?: string
    action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
    return (
        <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
            {icon && <div className="mb-2 text-4xl text-slate-500">{icon}</div>}
            <h3 className="text-lg font-medium text-slate-300">{title}</h3>
            {description && <p className="max-w-sm text-sm text-slate-500">{description}</p>}
            {action && <div className="mt-2">{action}</div>}
        </div>
    )
}