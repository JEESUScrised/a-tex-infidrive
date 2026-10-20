export interface DriveFolder {
    id: string;
    parent_id: string;
    name: string;
    created_at: string;
}

export interface DriveFile {
    id: string;
    folder_id: string;
    name: string;
    mime: string;
    size: number;
    sha256: string;
    provider: string;
    created_at: string;
    updated_at: string;
    deleted_at?: string;
    status: string;
}

export interface DriveListing {
    folder_id: string;
    folders: DriveFolder[];
    files: DriveFile[];
}

export interface ShareLink {
    id: string;
    file_id: string;
    token: string;
    url: string;
    expires_at?: string;
    max_downloads: number;
    download_count: number;
    revoked: boolean;
    created_at: string;
    file_name?: string;
}

export interface ProviderStatus {
    provider: string;
    ok: boolean;
    error?: string;
    usage_bytes?: number;
}
