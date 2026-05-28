import type {NextConfig} from 'next';
import createNextIntlPlugin from 'next-intl/plugin';

const withNextIntl = createNextIntlPlugin('./src/i18n/request.ts');

const backendURL = process.env.BACKEND_URL ?? 'http://localhost:8080';

const nextConfig: NextConfig = {
  // Standalone output produces a self-contained .next/standalone tree the
  // production Dockerfile copies. Saves ~200MB vs. shipping node_modules.
  output: 'standalone',
  async rewrites() {
    return [
      {source: '/api/:path*', destination: `${backendURL}/api/:path*`},
      {source: '/banners/:path*', destination: `${backendURL}/banners/:path*`},
    ];
  },
};

export default withNextIntl(nextConfig);
