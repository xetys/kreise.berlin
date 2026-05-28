// Fetch helpers around the backend API.
//
// Backend lives at /api/* (proxied via next.config.ts). Browsers see same-origin
// so session + CSRF cookies are sent automatically with `credentials: 'include'`.
//
// CSRF: the backend issues a non-HttpOnly cookie `tg_csrf` on any GET. For
// state-changing requests we read that cookie and copy its value into the
// `X-CSRF-Token` header. The first state-changing call after a fresh load
// triggers `ensureCsrf()` to make sure the cookie exists.

const CSRF_COOKIE = 'tg_csrf';

export class APIError extends Error {
  status: number;
  code: string;
  developerMessage: string;
  constructor(status: number, code: string, devMsg: string) {
    super(`${status} ${code}: ${devMsg}`);
    this.status = status;
    this.code = code;
    this.developerMessage = devMsg;
  }
}

function getCookie(name: string): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const match = document.cookie.match(
    new RegExp('(?:^|; )' + name.replace(/[.$?*|{}()[\]\\/+^]/g, '\\$&') + '=([^;]*)')
  );
  return match ? decodeURIComponent(match[1]) : undefined;
}

let csrfBootstrapped = false;

async function ensureCsrf(): Promise<string> {
  let token = getCookie(CSRF_COOKIE);
  if (!token || !csrfBootstrapped) {
    await fetch('/api/csrf', {credentials: 'include'});
    csrfBootstrapped = true;
    token = getCookie(CSRF_COOKIE);
  }
  if (!token) {
    throw new APIError(0, 'csrf_unavailable', 'CSRF cookie not set after bootstrap');
  }
  return token;
}

type Method = 'GET' | 'POST' | 'PATCH' | 'DELETE' | 'PUT' | 'HEAD';

interface RequestOptions {
  method?: Method;
  body?: unknown;
  headers?: Record<string, string>;
}

export async function api<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const method = opts.method ?? 'GET';
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(opts.headers ?? {}),
  };

  const init: RequestInit = {
    method,
    credentials: 'include',
    headers,
  };

  if (method !== 'GET' && method !== 'HEAD') {
    const token = await ensureCsrf();
    headers['X-CSRF-Token'] = token;
  }

  if (opts.body !== undefined) {
    if (opts.body instanceof FormData) {
      init.body = opts.body;
      // Let the browser set Content-Type with boundary.
    } else {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(opts.body);
    }
  }

  const res = await fetch(path, init);

  if (res.status === 204) {
    return undefined as T;
  }

  const text = await res.text();
  let payload: unknown = undefined;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }

  if (!res.ok) {
    if (payload && typeof payload === 'object' && 'error' in payload) {
      const p = payload as {error?: string; developer_message?: string};
      throw new APIError(res.status, p.error ?? 'unknown', p.developer_message ?? '');
    }
    throw new APIError(res.status, 'http_error', String(payload ?? res.statusText));
  }

  return payload as T;
}
