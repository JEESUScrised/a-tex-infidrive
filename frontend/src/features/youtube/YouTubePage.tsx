import {Button, Card, Flex, Label, Loader, Table, Text, useToaster} from '@gravity-ui/uikit';
import type {TableColumnConfig} from '@gravity-ui/uikit';
import {useEffect, useMemo, useState} from 'react';
import {useSearchParams} from 'react-router-dom';

import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {
    useFilesQuery,
    useYouTubeAuthMutation,
    useYouTubeDisconnectMutation,
    useYouTubeQuery,
} from '@/hooks/useApiQueries';
import {formatDate} from '@/lib/format';
import {ApiError, type StoredFile} from '@/types/api';

export function YouTubePage() {
    const toaster = useToaster();
    const [params, setParams] = useSearchParams();
    const youtubeQuery = useYouTubeQuery();
    const filesQuery = useFilesQuery();
    const authMutation = useYouTubeAuthMutation();
    const disconnectMutation = useYouTubeDisconnectMutation();
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const connected = params.get('connected') || params.get('youtube');
        const err = params.get('error') || params.get('youtube_error');
        if (connected === '1' || connected === 'connected') {
            toaster.add({name: 'yt-ok', title: 'Google-аккаунт подключён', theme: 'success'});
            setParams({}, {replace: true});
        }
        if (err) {
            setError(err);
            setParams({}, {replace: true});
        }
    }, [params, setParams, toaster]);

    const linked = useMemo(
        () => (filesQuery.data ?? []).filter((f) => f.youtube_video_id || f.youtube_error),
        [filesQuery.data],
    );

    const columns: TableColumnConfig<StoredFile>[] = [
        {
            id: 'original_name',
            name: 'Файл',
            primary: true,
            template: (f) => f.original_name,
        },
        {
            id: 'youtube_video_id',
            name: 'Video ID',
            template: (f) => f.youtube_video_id || '—',
        },
        {
            id: 'youtube_upload_status',
            name: 'Загрузка',
            template: (f) => f.youtube_upload_status || '—',
        },
        {
            id: 'youtube_processing_status',
            name: 'Обработка',
            template: (f) => f.youtube_processing_status || '—',
        },
        {
            id: 'youtube_privacy_status',
            name: 'Privacy',
            template: (f) => f.youtube_privacy_status || '—',
        },
        {
            id: 'youtube_uploaded_at',
            name: 'Загружено',
            template: (f) => formatDate(f.youtube_uploaded_at),
        },
        {
            id: 'link',
            name: '',
            template: (f) =>
                f.youtube_url ? (
                    <Button size="s" view="flat-info" href={f.youtube_url} target="_blank">
                        Открыть
                    </Button>
                ) : f.youtube_error ? (
                    <Text color="danger">{f.youtube_error}</Text>
                ) : null,
        },
    ];

    const data = youtubeQuery.data;

    return (
        <div className="yt-page">
            <PageHeader
                title="YouTube"
                description="Официальный YouTube Data API v3. Секреты OAuth в интерфейсе не показываются. Загрузка только по явной кнопке на странице Файлы."
            />
            <ErrorAlert message={error} />
            {youtubeQuery.isLoading ? (
                <Loader size="l" />
            ) : youtubeQuery.isError ? (
                <ErrorAlert
                    message={
                        youtubeQuery.error instanceof Error
                            ? youtubeQuery.error.message
                            : 'Не удалось получить статус YouTube'
                    }
                />
            ) : (
                <Card view="outlined" className="yt-card">
                    <Flex direction="column" gap={3}>
                        <Flex gap={2} alignItems="center" wrap>
                            <Label theme={data?.enabled ? 'success' : 'unknown'}>
                                {data?.enabled ? 'OAuth настроен' : 'OAuth не настроен'}
                            </Label>
                            <Label theme={data?.connected ? 'success' : 'warning'}>
                                {data?.connected ? 'Аккаунт подключён' : 'Аккаунт не подключён'}
                            </Label>
                        </Flex>
                        <Text>{data?.message}</Text>
                        {data?.channel ? (
                            <Text>
                                Канал: <b>{data.channel.title}</b> ({data.channel.id})
                            </Text>
                        ) : null}
                        <Flex gap={2}>
                            {data?.enabled && !data.connected ? (
                                <Button
                                    view="action"
                                    loading={authMutation.isPending}
                                    onClick={async () => {
                                        setError(null);
                                        try {
                                            await authMutation.mutateAsync();
                                        } catch (err) {
                                            setError(
                                                err instanceof ApiError
                                                    ? err.message
                                                    : 'Не удалось начать OAuth',
                                            );
                                        }
                                    }}
                                >
                                    Подключить Google
                                </Button>
                            ) : null}
                            {data?.connected ? (
                                <Button
                                    view="outlined-danger"
                                    loading={disconnectMutation.isPending}
                                    onClick={async () => {
                                        setError(null);
                                        try {
                                            await disconnectMutation.mutateAsync();
                                            toaster.add({
                                                name: 'disc',
                                                title: 'Аккаунт отключён',
                                                theme: 'warning',
                                            });
                                        } catch (err) {
                                            setError(
                                                err instanceof ApiError
                                                    ? err.message
                                                    : 'Не удалось отключить',
                                            );
                                        }
                                    }}
                                >
                                    Отключить аккаунт
                                </Button>
                            ) : null}
                        </Flex>
                    </Flex>
                </Card>
            )}

            <Text variant="subheader-2" style={{marginTop: 24}}>
                Файлы на YouTube
            </Text>
            {!linked.length ? (
                <EmptyState
                    title="Пока нет загрузок"
                    description="Сначала закодируйте файл в MP4, подключите аккаунт и нажмите «YouTube» на странице Файлы."
                />
            ) : (
                <Table className="yt-table" data={linked} columns={columns} getRowId={(f) => f.id} width="max" wordWrap />
            )}
        </div>
    );
}
