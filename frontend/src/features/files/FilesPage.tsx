import {Loader, useToaster} from '@gravity-ui/uikit';
import {useState} from 'react';

import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {FileUploadZone} from '@/features/files/FileUploadZone';
import {FilesTable} from '@/features/files/FilesTable';
import {
    useEncodeMutation,
    useFilesQuery,
    useUploadMutation,
    useYouTubeQuery,
    useYouTubeUploadMutation,
} from '@/hooks/useApiQueries';
import {ApiError} from '@/types/api';

export function FilesPage() {
    const toaster = useToaster();
    const filesQuery = useFilesQuery();
    const youtubeQuery = useYouTubeQuery();
    const uploadMutation = useUploadMutation();
    const encodeMutation = useEncodeMutation();
    const ytUploadMutation = useYouTubeUploadMutation();
    const [busyId, setBusyId] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    const youtubeReady = Boolean(youtubeQuery.data?.enabled && youtubeQuery.data.connected);

    const onFiles = async (picked: File[]) => {
        setError(null);
        for (const file of picked) {
            try {
                await uploadMutation.mutateAsync(file);
                toaster.add({name: 'upload', title: 'Файл сохранён', content: file.name, theme: 'success'});
            } catch (err) {
                const message = err instanceof ApiError ? err.message : 'Не удалось загрузить файл';
                setError(message);
                toaster.add({name: 'upload-err', title: 'Ошибка загрузки', content: message, theme: 'danger'});
            }
        }
    };

    const onEncode = async (id: string) => {
        setError(null);
        setBusyId(id);
        try {
            await encodeMutation.mutateAsync(id);
            toaster.add({name: 'encode', title: 'Кодирование поставлено в очередь', theme: 'success'});
        } catch (err) {
            const message = err instanceof ApiError ? err.message : 'Не удалось начать кодирование';
            setError(message);
        } finally {
            setBusyId(null);
        }
    };

    const onYouTube = async (id: string) => {
        setError(null);
        setBusyId(id);
        try {
            await ytUploadMutation.mutateAsync(id);
            toaster.add({name: 'yt', title: 'Загрузка на YouTube в очереди', theme: 'success'});
        } catch (err) {
            const message = err instanceof ApiError ? err.message : 'Не удалось начать загрузку';
            setError(message);
        } finally {
            setBusyId(null);
        }
    };

    return (
        <div className="yt-page">
            <PageHeader
                title="Файлы"
                description="Загрузка исходников, кодирование в MP4 и действия над готовыми роликами."
            />
            <FileUploadZone disabled={uploadMutation.isPending} onFiles={onFiles} />
            <ErrorAlert message={error} />
            {filesQuery.isLoading ? (
                <Loader size="l" />
            ) : filesQuery.isError ? (
                <ErrorAlert
                    message={
                        filesQuery.error instanceof Error
                            ? filesQuery.error.message
                            : 'Не удалось получить список файлов'
                    }
                />
            ) : !filesQuery.data?.length ? (
                <EmptyState
                    title="Файлов пока нет"
                    description="Перетащите файл в область выше или выберите его через диалог."
                />
            ) : (
                <FilesTable
                    files={filesQuery.data}
                    youtubeReady={youtubeReady}
                    busyId={busyId}
                    onEncode={onEncode}
                    onYouTube={onYouTube}
                />
            )}
        </div>
    );
}
