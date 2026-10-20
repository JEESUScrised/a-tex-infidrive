import {apiRequest} from '@/api/client';
import type {AuthMe, HealthStatus} from '@/types/api';
import type {DriveFile, DriveFolder, DriveListing, ProviderStatus, ShareLink} from '@/types/drive';

export const api = {
    health: () => apiRequest<HealthStatus>('/api/health'),
    system: () => apiRequest<HealthStatus>('/api/system'),
    authMe: () => apiRequest<AuthMe>('/api/auth/me'),
    login: (username: string, password: string) =>
        apiRequest<AuthMe>('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({username, password}),
        }),
    logout: () => apiRequest<{status: string}>('/api/auth/logout', {method: 'POST'}),

    listDrive: (folderId = '') =>
        apiRequest<DriveListing>(`/api/drive${folderId ? `?folder_id=${encodeURIComponent(folderId)}` : ''}`),

    createFolder: (name: string, parentId = '') =>
        apiRequest<DriveFolder>('/api/drive/folders', {
            method: 'POST',
            body: JSON.stringify({name, parent_id: parentId}),
        }),

    uploadFile: async (file: File, folderId = '') => {
        const form = new FormData();
        form.append('file', file);
        if (folderId) {
            form.append('folder_id', folderId);
        }
        return apiRequest<DriveFile>('/api/drive/upload', {method: 'POST', body: form});
    },

    trashFile: (id: string) =>
        apiRequest<{status: string}>(`/api/drive/${id}`, {method: 'DELETE'}),

    listTrash: () => apiRequest<DriveFile[]>('/api/drive/trash'),

    restoreFile: (id: string) =>
        apiRequest<{status: string}>(`/api/drive/trash/${id}/restore`, {method: 'POST'}),

    purgeFile: (id: string) =>
        apiRequest<{status: string}>(`/api/drive/trash/${id}`, {method: 'DELETE'}),

    createShare: (id: string, ttlHours = 0, maxDownloads = 0) =>
        apiRequest<ShareLink>(`/api/drive/${id}/share`, {
            method: 'POST',
            body: JSON.stringify({ttl_hours: ttlHours, max_downloads: maxDownloads}),
        }),

    listShares: () => apiRequest<ShareLink[]>('/api/drive/shares'),

    revokeShare: (id: string) =>
        apiRequest<{status: string}>(`/api/drive/shares/${id}`, {method: 'DELETE'}),

    providerStatus: () => apiRequest<ProviderStatus>('/api/drive/provider'),
};

export const driveUrls = {
    download: (id: string) => `/api/drive/${id}/download`,
};
