import {Button, Flex, Loader, Table, Text} from '@gravity-ui/uikit';

import {EmptyState} from '@/components/EmptyState';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {usePurgeMutation, useRestoreMutation, useTrashQuery} from '@/hooks/useApiQueries';
import {formatBytes, formatDate} from '@/lib/format';
import type {DriveFile} from '@/types/drive';

export function TrashPage() {
    const trash = useTrashQuery();
    const restore = useRestoreMutation();
    const purge = usePurgeMutation();

    const columns = [
        {
            id: 'name',
            name: 'Имя',
            template: (row: DriveFile) => <Text className="yt-ellipsis">{row.name}</Text>,
        },
        {
            id: 'size',
            name: 'Размер',
            template: (row: DriveFile) => formatBytes(row.size),
        },
        {
            id: 'deleted',
            name: 'Удалён',
            template: (row: DriveFile) => formatDate(row.deleted_at || row.updated_at),
        },
        {
            id: 'actions',
            name: '',
            template: (row: DriveFile) => (
                <Flex gap={1}>
                    <Button view="outlined" size="s" onClick={() => restore.mutate(row.id)}>
                        Восстановить
                    </Button>
                    <Button view="outlined-danger" size="s" onClick={() => purge.mutate(row.id)}>
                        Удалить навсегда
                    </Button>
                </Flex>
            ),
        },
    ];

    return (
        <div className="yt-page">
            <PageHeader title="Корзина" />
            {trash.isLoading ? (
                <Loader size="l" />
            ) : trash.isError ? (
                <ErrorAlert
                    message={trash.error instanceof Error ? trash.error.message : 'Ошибка'}
                />
            ) : !trash.data?.length ? (
                <EmptyState title="Корзина пуста" />
            ) : (
                <Table
                    className="yt-table"
                    data={trash.data}
                    columns={columns}
                    getRowId={(r) => r.id}
                    wordWrap
                    width="max"
                />
            )}
        </div>
    );
}
