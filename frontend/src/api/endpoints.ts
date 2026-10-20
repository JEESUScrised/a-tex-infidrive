import {apiRequest} from '@/api/client';
import type {
    HealthStatus,
    Job,
    StoredFile,
    YouTubeFileStatus,
    YouTubeStatus,
} from '@/types/api';

export const api = {
    health: () => apiRequest<HealthStatus>('/api/health'),

    listFiles: () => apiRequest<StoredFile[]>('/api/files'),

    getFile: (id: string) => apiRequest<StoredFile>(`/api/files/${id}`),

    uploadFile: async (file: File) => {
        const form = new FormData();
        form.append('file', file);
        return apiRequest<StoredFile>('/api/files', {method: 'POST', body: form});
    },

    encodeFile: (id: string) =>
        apiRequest<Job>(`/api/files/${id}/encode`, {method: 'POST'}),

    uploadToYouTube: (id: string) =>
        apiRequest<Job>(`/api/files/${id}/youtube`, {method: 'POST'}),

    fileYouTubeStatus: (id: string) =>
        apiRequest<YouTubeFileStatus>(`/api/files/${id}/youtube`),

    recoverFromMp4: async (file: File) => {
        const form = new FormData();
        form.append('file', file);
        return apiRequest<Job>('/api/recover', {method: 'POST', body: form});
    },

    listJobs: () => apiRequest<Job[]>('/api/jobs'),

    getJob: (id: string) => apiRequest<Job>(`/api/jobs/${id}`),

    cancelJob: (id: string) =>
        apiRequest<Job>(`/api/jobs/${id}/cancel`, {method: 'POST'}),

    youtubeStatus: () => apiRequest<YouTubeStatus>('/api/youtube'),

    youtubeAuthStart: () => apiRequest<{auth_url: string}>('/api/youtube/auth/start'),

    youtubeDisconnect: () =>
        apiRequest<{status: string}>('/api/youtube/disconnect', {method: 'POST'}),
};

export const fileUrls = {
    original: (id: string) => `/api/files/${id}/download`,
    video: (id: string) => `/api/files/${id}/video`,
    jobResult: (id: string) => `/api/jobs/${id}/download`,
};
