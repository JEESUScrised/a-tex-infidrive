import {ApiError, type ApiErrorBody} from '@/types/api';

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
    const response = await fetch(path, init);
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
