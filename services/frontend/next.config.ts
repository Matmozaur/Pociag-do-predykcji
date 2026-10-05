import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
    async rewrites() {
        return [
            {
                source: '/bff/:path*',
                destination: `${process.env.API_URL ?? 'http://localhost:8080'}/:path*`,
            },
        ]
    },
}

export default nextConfig
