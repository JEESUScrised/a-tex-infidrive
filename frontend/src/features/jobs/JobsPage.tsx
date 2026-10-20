import {Button, Flex, Label, Loader, Progress, Table, Text, useToaster} from '@gravity-ui/uikit';
import type {TableColumnConfig} from '@gravity-ui/uikit';
import {useMemo, useState} from 'react';

import {fileUrls} from '@/api/endpoints';
import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {useCancelJobMutation, useFilesQuery, useJobsQuery} from '@/hooks/useApiQueries';
import {canCancelJob, formatDate, jobKindLabel} from '@/lib/format';
import {ApiError, type Job} from '@/types/api';

function statusTheme(status: string): 'success' | 'danger' | 'warning' | 'info' | 'unknown' {
    switch (status) {
        case 'succeeded':
        case 'completed':
            return 'success';
        case 'failed':
            return 'danger';
        case 'cancelled':
            return 'unknown';
        case 'queued':
            return 'warning';
        default:
            return 'info';
    }
}

export function JobsPage() {
    const toaster = useToaster();
    const jobsQuery = useJobsQuery();
    const filesQuery = useFilesQuery();
    const cancelMutation = useCancelJobMutation();
    const [error, setError] = useState<string | null>(null);

    const names = useMemo(() => {
        const map = new Map<string, string>();
        for (const f of filesQuery.data ?? []) {
            map.set(f.id, f.original_name);
        }
        return map;
    }, [filesQuery.data]);

    const columns: TableColumnConfig<Job>[] = [
        {
            id: 'kind',
            name: 'Тип',
            width: 140,
            template: (j) => jobKindLabel(j.kind),
        },
        {
            id: 'file',
            name: 'Файл',
            width: 180,
            template: (j) => (
                <Text className="yt-ellipsis" title={names.get(j.file_id) ?? j.file_id}>
                    {names.get(j.file_id) || j.output_name || j.file_id || '—'}
                </Text>
            ),
        },
        {
            id: 'status',
            name: 'Статус',
            width: 120,
            template: (j) => (
                <Label theme={statusTheme(j.status)} size="xs">
                    {j.status}
                </Label>
            ),
        },
        {
            id: 'progress',
            name: 'Прогресс',
            width: 160,
            template: (j) => (
                <Flex direction="column" gap={1}>
                    <Text variant="caption-2">{j.progress}%</Text>
                    <Progress value={j.progress} size="s" />
                </Flex>
            ),
        },
        {
            id: 'created_at',
            name: 'Создано',
            width: 150,
            template: (j) => formatDate(j.created_at),
        },
        {
            id: 'result',
            name: 'Результат',
            width: 220,
            template: (j) => (
                <Text color={j.error ? 'danger' : 'secondary'} className="yt-ellipsis" title={j.error || j.message}>
                    {j.error || j.message || '—'}
                </Text>
            ),
        },
        {
            id: 'actions',
            name: '',
            width: 160,
            template: (j) => (
                <Flex gap={1}>
                    {canCancelJob(j.status) ? (
                        <Button
                            size="s"
                            view="outlined-danger"
                            loading={cancelMutation.isPending}
                            onClick={async () => {
                                setError(null);
                                try {
                                    await cancelMutation.mutateAsync(j.id);
                                    toaster.add({
                                        name: 'cancel',
                                        title: 'Задание отменено',
                                        theme: 'warning',
                                    });
                                } catch (err) {
                                    setError(err instanceof ApiError ? err.message : 'Не удалось отменить');
                                }
                            }}
                        >
                            Отмена
                        </Button>
                    ) : null}
                    {j.status === 'succeeded' ? (
                        <Button size="s" view="flat" href={fileUrls.jobResult(j.id)}>
                            Скачать
                        </Button>
                    ) : null}
                </Flex>
            ),
        },
    ];

    return (
        <div className="yt-page">
            <PageHeader
                title="Задания"
                description="Очередь кодирования, восстановления и загрузки на YouTube. Опрос централизован через TanStack Query."
            />
            <ErrorAlert message={error} />
            {jobsQuery.isLoading ? (
                <Loader size="l" />
            ) : jobsQuery.isError ? (
                <ErrorAlert
                    message={
                        jobsQuery.error instanceof Error
                            ? jobsQuery.error.message
                            : 'Не удалось получить задания'
                    }
                />
            ) : !jobsQuery.data?.length ? (
                <EmptyState title="Заданий нет" description="Запустите кодирование или восстановление на странице Файлы." />
            ) : (
                <Table
                    className="yt-table"
                    data={jobsQuery.data}
                    columns={columns}
                    getRowId={(item) => item.id}
                    wordWrap
                    width="max"
                />
            )}
        </div>
    );
}
