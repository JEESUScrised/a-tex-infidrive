import {Button, Flex, Loader, Table, Text} from '@gravity-ui/uikit';

import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {useRevokeShareMutation, useSharesQuery} from '@/hooks/useApiQueries';
import {formatDate} from '@/lib/format';
import type {ShareLink} from '@/types/drive';

export function SharesPage() {
    const shares = useSharesQuery();
    const revoke = useRevokeShareMutation();

    const columns = [
        {
            id: 'file',
            name: 'Файл',
            template: (row: ShareLink) => row.file_name || row.file_id,
        },
        {
            id: 'url',
            name: 'Ссылка',
            template: (row: ShareLink) => (
                <Text className="yt-ellipsis">{row.url || `/s/${row.token}`}</Text>
            ),
        },
        {
            id: 'meta',
            name: 'Ограничения',
            template: (row: ShareLink) =>
                `${row.download_count}${row.max_downloads ? `/${row.max_downloads}` : ''} · ${row.expires_at ? formatDate(row.expires_at) : 'без срока'}`,
        },
        {
            id: 'actions',
            name: '',
            template: (row: ShareLink) => (
                <Flex gap={1}>
                    <Button
                        view="outlined"
                        size="s"
                        onClick={() =>
                            navigator.clipboard.writeText(row.url || `${window.location.origin}/s/${row.token}`)
                        }
                    >
                        Копировать
                    </Button>
                    <Button
                        view="outlined-danger"
                        size="s"
                        loading={revoke.isPending}
                        onClick={() => revoke.mutate(row.id)}
                    >
                        Отозвать
                    </Button>
                </Flex>
            ),
        },
    ];

    return (
        <div className="yt-page">
            <PageHeader title="Общие ссылки" />
            {shares.isLoading ? (
                <Loader size="l" />
            ) : shares.isError ? (
                <ErrorAlert
                    message={shares.error instanceof Error ? shares.error.message : 'Ошибка'}
                />
            ) : !shares.data?.length ? (
                <EmptyState title="Ссылок нет" />
            ) : (
                <Table
                    className="yt-table"
                    data={shares.data}
                    columns={columns}
                    getRowId={(r) => r.id}
                    wordWrap
                    width="max"
                />
            )}
        </div>
    );
}
