import {ApiError, type ApiErrorBody} from '@/types/api';

function readCookie(name: string): string | undefined {
    const prefix = `${name}=`;
    const parts = document.cookie.split(';');
    for (const part of parts) {
        const trimmed = part.trim();
        if (trimmed.startsWith(prefix)) {
            return decodeURIComponent(trimmed.slice(prefix.length));
        }
    }
    return undefined;
}

async function ensureCsrf(forceRefresh = false): Promise<string> {
    if (!forceRefresh) {
        const existing = readCookie('infidrive_csrf');
        if (existing) {
            return existing;
        }
    }
    const response = await fetch('/api/auth/csrf', {credentials: 'include', cache: 'no-store'});
    if (!response.ok) {
        return readCookie('infidrive_csrf') ?? '';
    }
    const body = (await response.json()) as {csrf_token?: string};
    const token = body.csrf_token ?? '';
    // Страховка double-submit: если Set-Cookie не сохранился, выставляем сами.
    if (token) {
        const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
        document.cookie = `infidrive_csrf=${encodeURIComponent(token)}; Path=/; SameSite=Lax${secure}`;
    }
    return token || readCookie('infidrive_csrf') || '';
}

async function parseBody(response: Response): Promise<unknown> {
    const text = await response.text();
    if (!text) {
        return null;
    }
    try {
        return JSON.parse(text) as unknown;
    } catch {
        return {error: text} satisfies ApiErrorBody;
    }
}

export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
    const headers = new Headers(init?.headers);
    const method = (init?.method ?? 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
        // Для login всегда свежий CSRF — иначе рассинхрон cookie/заголовка даёт 403.
        const force = path.includes('/auth/login') || path.includes('/auth/logout');
        const csrf = await ensureCsrf(force);
        if (csrf) {
            headers.set('X-CSRF-Token', csrf);
        }
    }
    if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json');
    }
    const response = await fetch(path, {
        ...init,
        headers,
        credentials: 'include',
    });
    const body = await parseBody(response);
    if (!response.ok) {
        const message =
            body && typeof body === 'object' && 'error' in body && typeof (body as ApiErrorBody).error === 'string'
                ? (body as ApiErrorBody).error
                : response.statusText || 'Ошибка запроса';
        throw new ApiError(response.status, message);
    }
    return body as T;
}

export function downloadUrl(path: string): string {
    return path;
}
