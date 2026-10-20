export function formatBytes(n: number): string {
    if (n < 1024) {
        return `${n} Б`;
    }
    if (n < 1024 * 1024) {
        return `${(n / 1024).toFixed(1)} КБ`;
    }
    return `${(n / (1024 * 1024)).toFixed(1)} МБ`;
}

export function formatDate(iso: string): string {
    if (!iso) {
        return '—';
    }
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) {
        return iso;
    }
    return d.toLocaleString('ru-RU', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
    });
}

export function shortHash(hash: string): string {
    if (hash.length <= 16) {
        return hash;
    }
    return `${hash.slice(0, 8)}…${hash.slice(-6)}`;
}

export function jobKindLabel(kind: string): string {
    switch (kind) {
        case 'encode':
            return 'Кодирование';
        case 'decode':
            return 'Восстановление';
        case 'youtube_upload':
            return 'YouTube';
        default:
            return kind;
    }
}

export function canCancelJob(status: string): boolean {
    return (
        status === 'queued' ||
        status === 'running' ||
        status === 'uploading' ||
        status === 'uploaded' ||
        status === 'processing'
    );
}

export function fileStatusLabel(file: {
    has_video: boolean;
    youtube_video_id: string;
    youtube_upload_status: string;
    youtube_error: string;
}): {text: string; theme: 'success' | 'warning' | 'danger' | 'info' | 'unknown'} {
    if (file.youtube_error) {
        return {text: 'Ошибка YouTube', theme: 'danger'};
    }
    if (file.youtube_video_id) {
        return {text: 'На YouTube', theme: 'success'};
    }
    if (file.has_video) {
        return {text: 'MP4 готов', theme: 'info'};
    }
    return {text: 'Исходный файл', theme: 'unknown'};
}
