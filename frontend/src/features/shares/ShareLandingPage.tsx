import {Button, Card, Flex, Loader, Text} from '@gravity-ui/uikit';
import {useQuery} from '@tanstack/react-query';
import {useParams} from 'react-router-dom';

import {ErrorAlert} from '@/components/ErrorAlert';
import {formatBytes} from '@/lib/format';

type PublicShareView = {
    file_name: string;
    file_size: number;
    expires_at?: string;
    download_available: boolean;
    retrieval_hint?: string;
};

export function ShareLandingPage() {
    const {token = ''} = useParams();
    const meta = useQuery({
        queryKey: ['public-share', token],
        enabled: Boolean(token),
        queryFn: async () => {
            const response = await fetch(`/api/public/share/${encodeURIComponent(token)}`);
            if (!response.ok) {
                const text = await response.text();
                throw new Error(text || response.statusText);
            }
            return (await response.json()) as PublicShareView;
        },
    });

    return (
        <div className="yt-page yt-share-landing">
            <Card view="outlined" className="yt-card">
                <Flex direction="column" gap={4}>
                    <Text variant="header-1">InfiDrive</Text>
                    {meta.isLoading ? <Loader size="l" /> : null}
                    {meta.isError ? (
                        <ErrorAlert
                            message={meta.error instanceof Error ? meta.error.message : 'Ссылка недоступна'}
                        />
                    ) : null}
                    {meta.data ? (
                        <>
                            <Flex direction="column" gap={1}>
                                <Text variant="subheader-2">{meta.data.file_name}</Text>
                                <Text color="secondary">{formatBytes(meta.data.file_size)}</Text>
                                {meta.data.expires_at ? (
                                    <Text color="secondary">Действует до {meta.data.expires_at}</Text>
                                ) : null}
                            </Flex>
                            {meta.data.download_available ? (
                                <Button view="action" size="l" href={`/s/${token}/download`}>
                                    Скачать
                                </Button>
                            ) : (
                                <Text color="secondary">
                                    {meta.data.retrieval_hint ||
                                        'Скачивание временно недоступно'}
                                </Text>
                            )}
                        </>
                    ) : null}
                </Flex>
            </Card>
        </div>
    );
}
