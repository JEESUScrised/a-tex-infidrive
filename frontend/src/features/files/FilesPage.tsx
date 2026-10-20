import {Button, Flex, Loader, Table, Text, TextInput, useToaster} from '@gravity-ui/uikit';
import {useMemo, useState} from 'react';

import {driveUrls} from '@/api/endpoints';
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

export function FilesPage() {
    const toaster = useToaster();
    const [folderId, setFolderId] = useState('');
    const [folderName, setFolderName] = useState('');
    const [error, setError] = useState<string | null>(null);
    const drive = useDriveQuery(folderId);
    const upload = useUploadMutation();
    const createFolder = useCreateFolderMutation();
    const trash = useTrashMutation();
    const share = useShareMutation();

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
                            <Button view="outlined" size="s" href={driveUrls.download(row.id)}>
                                Скачать
                            </Button>
                            <Button
                                view="outlined"
                                size="s"
                                loading={share.isPending}
                                onClick={async () => {
                                    try {
                                        const link = await share.mutateAsync({id: row.id});
                                        await navigator.clipboard.writeText(link.url || `${window.location.origin}/s/${link.token}`);
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
        [share, toaster, trash],
    );

    const rows: Array<DriveFile | DriveFolder> = [
        ...(drive.data?.folders ?? []),
        ...(drive.data?.files ?? []),
    ];

    return (
        <div className="yt-page">
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
