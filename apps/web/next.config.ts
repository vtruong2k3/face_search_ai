import type { NextConfig } from "next";

/**
 * Deliberate browser security headers for the customer- and photographer-facing app.
 *
 * Notes on scope and the camera/file-input flows:
 * - `Permissions-Policy` grants `camera=(self)` so the public selfie-search page can
 *   call `getUserMedia`; all other powerful features are denied. The file-input
 *   fallback needs no permission. Removing `camera=(self)` would break selfie capture.
 * - The Content-Security-Policy is intentionally conservative rather than complete: it
 *   sets `frame-ancestors 'none'` (clickjacking), `base-uri 'self'` (base-tag injection),
 *   and `object-src 'none'` (legacy plugins). It does NOT set a restrictive `default-src`,
 *   `script-src`, or `connect-src`, because Next.js hydration relies on inline bootstrap
 *   scripts/styles and the app connects to env-driven API and MinIO origins (selfie POST,
 *   direct multipart upload, signed downloads). A full allow-list/nonce CSP is deferred to
 *   avoid breaking hydration, camera capture, uploads, and downloads; see docs/architecture.md.
 * - HSTS is set at the TLS-terminating edge (Caddy), not here, since this app may be served
 *   over plain HTTP in local development.
 */
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(self), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()",
  },
  {
    key: "Content-Security-Policy",
    value: "base-uri 'self'; object-src 'none'; frame-ancestors 'none'",
  },
];

const nextConfig: NextConfig = {
  output: "standalone",
  async headers() {
    return [
      {
        source: "/:path*",
        headers: securityHeaders,
      },
    ];
  },
};

export default nextConfig;
