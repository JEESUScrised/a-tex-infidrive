import {Button, Card, Flex, Loader, Progress, Table, Text, TextInput, useToaster} from '@gravity-ui/uikit';
import {useCallback, useMemo, useRef, useState} from 'react';

import {api} from '@/api/endpoints';
import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {FileUploadZone} from '@/features/files/FileUploadZone';
import {
    useCreateFolderMutation,
    useDriveQuery,
    useShareMutation,
    useTrashMutation,
    useUploadMutation,
} from '@/hooks/useApiQueries';
import {formatBytes, formatDate} from '@/lib/format';
import {ApiError} from '@/types/api';
import type {DriveFile, DriveFolder} from '@/types/drive';

function triggerBlobDownload(blob: Blob, name: string) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
}

async function sleep(ms: number) {
    await new Promise((r) => setTimeout(r, ms));
}

export function FilesPage() {
    const toaster = useToaster();
    const [folderId, setFolderId] = useState('');
    const [folderName, setFolderName] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [restoreId, setRestoreId] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [progress, setProgress] = useState<{percent: number; message: string} | null>(null);
    const restoreInput = useRef<HTMLInputElement>(null);
    const drive = useDriveQuery(folderId);
    const upload = useUploadMutation();
    const createFolder = useCreateFolderMutation();
    const trash = useTrashMutation();
    const share = useShareMutation();

    const runRestoreJob = useCallback(
        async (file: DriveFile) => {
            setBusy(true);
            setError(null);
            setProgress({percent: 5, message: 'Подготовка файла…'});
            try {
                const job = await api.startRestoreJob(file.id);
                for (;;) {
                    const st = await api.restoreJobStatus(job.id);
                    setProgress({percent: st.percent || 0, message: st.message || 'Обработка…'});
                    if (st.state === 'done') {
                        const blob = await api.restoreJobFile(job.id);
                        triggerBlobDownload(blob, st.file_name || file.name);
                        toaster.add({name: 'dl', title: 'Файл восстановлен', theme: 'success'});
                        setProgress(null);
                        return;
                    }
                    if (st.state === 'error') {
                        throw new Error(st.error || 'Ошибка восстановления');
                    }
                    await sleep(800);
                }
            } catch (err) {
                const msg = err instanceof Error ? err.message : 'Ошибка';
                setError(msg);
                toaster.add({name: 'dl', title: 'Не удалось скачать', content: msg, theme: 'danger'});
                setProgress(null);
            } finally {
                setBusy(false);
            }
        },
        [toaster],
    );

    const columns = useMemo(
        () => [
            {
                id: 'name',
                name: 'Имя',
                template: (row: DriveFile | DriveFolder) => {
                    if ('mime' in row) {
                        return <Text className="yt-ellipsis">{row.name}</Text>;
                    }
                    return (
                        <Button view="flat-info" onClick={() => setFolderId(row.id)}>
                            📁 {row.name}
                        </Button>
                    );
                },
            },
            {
                id: 'size',
                name: 'Размер',
                template: (row: DriveFile | DriveFolder) =>
                    'mime' in row ? formatBytes(row.size) : '—',
            },
            {
                id: 'created',
                name: 'Добавлен',
                template: (row: DriveFile | DriveFolder) => formatDate(row.created_at),
            },
            {
                id: 'actions',
                name: '',
                template: (row: DriveFile | DriveFolder) => {
                    if (!('mime' in row)) {
                        return null;
                    }
                    return (
                        <Flex gap={1} wrap>
                            <Button
                                view="outlined"
                                size="s"
                                disabled={busy}
                                loading={busy}
                                onClick={() => runRestoreJob(row)}
                            >
                                Скачать
                            </Button>
                            <Button
                                view="outlined"
                                size="s"
                                onClick={() => {
                                    setRestoreId(row.id);
                                    restoreInput.current?.click();
                                }}
                            >
                                Из MP4
                            </Button>
                            <Button
                                view="outlined"
                                size="s"
                                loading={share.isPending}
                                onClick={async () => {
                                    try {
                                        const link = await share.mutateAsync({id: row.id});
                                        await navigator.clipboard.writeText(
                                            link.url || `${window.location.origin}/s/${link.token}`,
                                        );
                                        toaster.add({
                                            name: 'share',
                                            title: 'Ссылка скопирована',
                                            theme: 'success',
                                        });
                                    } catch (err) {
                                        setError(err instanceof ApiError ? err.message : 'Ошибка');
                                    }
                                }}
                            >
                                Поделиться
                            </Button>
                            <Button
                                view="outlined-danger"
                                size="s"
                                loading={trash.isPending}
                                onClick={() => trash.mutate(row.id)}
                            >
                                Удалить
                            </Button>
                        </Flex>
                    );
                },
            },
        ],
        [share, toaster, trash, busy, runRestoreJob],
    );

    const rows: Array<DriveFile | DriveFolder> = [
        ...(drive.data?.folders ?? []),
        ...(drive.data?.files ?? []),
    ];

    return (
        <div className="yt-page">
            <input
                ref={restoreInput}
                type="file"
                accept="video/mp4,.mp4"
                hidden
                onChange={async (event) => {
                    const file = event.target.files?.[0];
                    const id = restoreId;
                    event.target.value = '';
                    setRestoreId(null);
                    if (!file || !id) {
                        return;
                    }
                    try {
                        const blob = await api.restoreFromMp4(id, file);
                        const name =
                            drive.data?.files.find((f) => f.id === id)?.name || 'restored.bin';
                        triggerBlobDownload(blob, name);
                        toaster.add({
                            name: 'restore',
                            title: 'Файл восстановлен',
                            theme: 'success',
                        });
                    } catch (err) {
                        setError(err instanceof Error ? err.message : 'Ошибка восстановления');
                    }
                }}
            />
            <PageHeader
                title="Мои файлы"
                actions={
                    <Flex gap={2}>
                        {folderId ? (
                            <Button view="outlined" onClick={() => setFolderId('')}>
                                В корень
                            </Button>
                        ) : null}
                    </Flex>
                }
            />
            <FileUploadZone
                disabled={upload.isPending}
                onFiles={async (files) => {
                    setError(null);
                    for (const file of files) {
                        try {
                            await upload.mutateAsync({file, folderId});
                        } catch (err) {
                            setError(err instanceof ApiError ? err.message : 'Ошибка загрузки');
                        }
                    }
                }}
            />
            <Flex gap={2} alignItems="end">
                <TextInput
                    label="Новая папка"
                    value={folderName}
                    onUpdate={setFolderName}
                    placeholder="Имя папки"
                />
                <Button
                    view="action"
                    disabled={!folderName.trim()}
                    loading={createFolder.isPending}
                    onClick={async () => {
                        try {
                            await createFolder.mutateAsync({name: folderName.trim(), parentId: folderId});
                            setFolderName('');
                        } catch (err) {
                            setError(err instanceof ApiError ? err.message : 'Ошибка');
                        }
                    }}
                >
                    Создать
                </Button>
            </Flex>
            <ErrorAlert message={error} />
            {progress ? (
                <Card view="outlined" className="yt-card">
                    <Flex direction="column" gap={2}>
                        <Text>{progress.message}</Text>
                        <Progress value={progress.percent} text={`${progress.percent}%`} />
                    </Flex>
                </Card>
            ) : null}
            {drive.isLoading ? (
                <Loader size="l" />
            ) : drive.isError ? (
                <ErrorAlert
                    message={drive.error instanceof Error ? drive.error.message : 'Ошибка'}
                />
            ) : rows.length === 0 ? (
                <EmptyState title="Папка пуста" />
            ) : (
                <Table
                    className="yt-table"
                    data={rows}
                    columns={columns}
                    getRowId={(item) => item.id}
                    wordWrap
                    width="max"
                />
            )}
        </div>
    );
}
