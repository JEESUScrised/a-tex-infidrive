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

async function ensureCsrf(): Promise<string> {
    const existing = readCookie('infidrive_csrf');
    if (existing) {
        return existing;
    }
    const response = await fetch('/api/auth/csrf', {credentials: 'include'});
    if (!response.ok) {
        return '';
    }
    const body = (await response.json()) as {csrf_token?: string};
    return body.csrf_token ?? readCookie('infidrive_csrf') ?? '';
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
        const csrf = await ensureCsrf();
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
