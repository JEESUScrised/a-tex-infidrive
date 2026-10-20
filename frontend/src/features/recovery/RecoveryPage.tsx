import {Button, Card, Flex, Label, Loader, Progress, Text, useToaster} from '@gravity-ui/uikit';
import {useMemo, useState} from 'react';

import {fileUrls} from '@/api/endpoints';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {FileUploadZone} from '@/features/files/FileUploadZone';
import {useJobsQuery, useRecoverMutation} from '@/hooks/useApiQueries';
import {formatDate} from '@/lib/format';
import {ApiError} from '@/types/api';

export function RecoveryPage() {
    const toaster = useToaster();
    const recoverMutation = useRecoverMutation();
    const jobsQuery = useJobsQuery();
    const [error, setError] = useState<string | null>(null);
    const [activeJobId, setActiveJobId] = useState<string | null>(null);

    const activeJob = useMemo(() => {
        if (!activeJobId || !jobsQuery.data) {
            return null;
        }
        return jobsQuery.data.find((j) => j.id === activeJobId) ?? null;
    }, [activeJobId, jobsQuery.data]);

    const onFiles = async (files: File[]) => {
        const file = files[0];
        if (!file) {
            return;
        }
        setError(null);
        setActiveJobId(null);
        try {
            const job = await recoverMutation.mutateAsync(file);
            setActiveJobId(job.id);
            toaster.add({
                name: 'recover',
                title: 'Восстановление запущено',
                content: file.name,
                theme: 'success',
            });
        } catch (err) {
            const message = err instanceof ApiError ? err.message : 'Не удалось начать восстановление';
            setError(message);
        }
    };

    const failed = activeJob?.status === 'failed' || activeJob?.status === 'cancelled';
    const succeeded = activeJob?.status === 'succeeded';

    return (
        <div className="yt-page">
            <PageHeader
                title="Восстановление"
                description="Импортируйте MP4 (в том числе скачанный из YouTube Studio). Повреждённый результат не выдаётся за успех: сервер сверяет SHA-256."
            />
            <FileUploadZone disabled={recoverMutation.isPending} onFiles={onFiles} />
            <ErrorAlert message={error} />

            {activeJob ? (
                <Card view="outlined" className="yt-card">
                    <Flex direction="column" gap={3}>
                        <Flex gap={2} alignItems="center">
                            <Text variant="subheader-2">Задание восстановления</Text>
                            <Label
                                theme={
                                    succeeded ? 'success' : failed ? 'danger' : 'info'
                                }
                            >
                                {activeJob.status}
                            </Label>
                        </Flex>
                        <Flex direction="column" gap={1}>
                            <Text variant="caption-2">{activeJob.progress}%</Text>
                            <Progress value={activeJob.progress} size="s" />
                        </Flex>
                        <Text color="secondary">Создано: {formatDate(activeJob.created_at)}</Text>
                        <Text>{activeJob.message || '—'}</Text>
                        {activeJob.error ? <Text color="danger">{activeJob.error}</Text> : null}
                        {jobsQuery.isFetching && !succeeded && !failed ? <Loader size="s" /> : null}
                        {succeeded ? (
                            <Flex direction="column" gap={2}>
                                <Text>
                                    Имя результата: <b>{activeJob.output_name || 'restored'}</b>
                                </Text>
                                <Text color="secondary">
                                    SHA-256 и размер проверены на сервере. Если хеш не совпал, задание
                                    завершилось бы ошибкой.
                                </Text>
                                <Button view="action" href={fileUrls.jobResult(activeJob.id)}>
                                    Скачать восстановленный файл
                                </Button>
                            </Flex>
                        ) : null}
                        {failed ? (
                            <Text color="danger">
                                Восстановление не удалось. Повреждённый файл не публикуется.
                            </Text>
                        ) : null}
                    </Flex>
                </Card>
            ) : (
                <Text color="secondary">
                    Выберите MP4, полученный из YTDrive или скачанный из YouTube Studio для своего ролика.
                </Text>
            )}
        </div>
    );
}
