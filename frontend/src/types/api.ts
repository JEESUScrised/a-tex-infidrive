export interface HealthStatus {
    status: string;
    version: string;
    app?: string;
    ffmpeg?: boolean;
    ffmpeg_path?: string;
    ffprobe_path?: string;
    ffmpeg_source?: string;
    ffmpeg_info?: string;
}

export interface AuthMe {
    authenticated: boolean;
    auth_required: boolean;
    username?: string;
    app_name?: string;
}

export interface StoredFile {
    id: string;
    original_name: string;
    mime: string;
    size: number;
    sha256: string;
    created_at: string;
    has_video: boolean;
    youtube_video_id: string;
    youtube_url: string;
    youtube_upload_status: string;
    youtube_processing_status: string;
    youtube_uploaded_at: string;
    youtube_error: string;
    youtube_privacy_status: string;
}

export type JobKind = 'encode' | 'decode' | 'youtube_upload' | string;

export type JobStatus =
    | 'queued'
    | 'running'
    | 'succeeded'
    | 'failed'
    | 'cancelled'
    | 'uploading'
    | 'uploaded'
    | 'processing'
    | 'completed'
    | string;

export interface Job {
    id: string;
    kind: JobKind;
    status: JobStatus;
    progress: number;
    message: string;
    error: string;
    file_id: string;
    output_name: string;
    created_at: string;
    updated_at: string;
}

export interface YouTubeChannel {
    id: string;
    title: string;
}

export interface YouTubeRetrievalMeta {
    name: string;
    available: boolean;
    message: string;
    automatic: boolean;
    manual_fallback: boolean;
}

export interface YouTubeStatus {
    enabled: boolean;
    connected: boolean;
    provider: string;
    message: string;
    channel: YouTubeChannel | null;
    redirect_uri?: string;
    privacy_status?: string;
    experimental_store?: boolean;
    oauth_credentials_set?: boolean;
    retrieval?: YouTubeRetrievalMeta;
}

export interface YouTubeFileStatus {
    file_id: string;
    youtube_video_id: string;
    youtube_url: string;
    youtube_upload_status: string;
    youtube_processing_status: string;
    youtube_uploaded_at: string;
    youtube_error: string;
    youtube_privacy_status: string;
}

export interface ApiErrorBody {
    error: string;
}

export class ApiError extends Error {
    readonly status: number;

    constructor(status: number, message: string) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
    }
}
