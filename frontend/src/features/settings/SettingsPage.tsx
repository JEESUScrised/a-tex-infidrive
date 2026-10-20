import {Button, Card, Flex, Label, Loader, Text} from '@gravity-ui/uikit';
import type {Theme} from '@gravity-ui/uikit';

import {useThemeControl} from '@/app/providers';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {api} from '@/api/endpoints';
import {useAuthQuery, useLogoutMutation, useProviderQuery} from '@/hooks/useApiQueries';
import {formatBytes} from '@/lib/format';
import {useQuery} from '@tanstack/react-query';

const themes: {id: Theme; label: string}[] = [
    {id: 'system', label: 'Системная'},
    {id: 'light', label: 'Светлая'},
    {id: 'dark', label: 'Тёмная'},
];

export function SettingsPage() {
    const auth = useAuthQuery();
    const logout = useLogoutMutation();
    const provider = useProviderQuery();
    const retrieval = useQuery({
        queryKey: ['retrieval'],
        queryFn: api.retrievalStatus,
        retry: false,
    });
    const {theme, setTheme} = useThemeControl();

    return (
        <div className="yt-page">
            <PageHeader title="Настройки" />

            <Card view="outlined" className="yt-card">
                <Flex direction="column" gap={3}>
                    <Text variant="subheader-2">Сессия</Text>
                    <Text color="secondary">
                        {auth.data?.username ? `Пользователь: ${auth.data.username}` : 'Локальный режим'}
                    </Text>
                    {auth.data?.auth_required ? (
                        <Button view="outlined" onClick={() => logout.mutate()} loading={logout.isPending}>
                            Выйти
                        </Button>
                    ) : null}
                </Flex>
            </Card>

            <Card view="outlined" className="yt-card">
                <Flex direction="column" gap={3}>
                    <Text variant="subheader-2">Тема</Text>
                    <Flex gap={2} wrap>
                        {themes.map((item) => (
                            <Button
                                key={item.id}
                                view={theme === item.id ? 'action' : 'outlined'}
                                onClick={() => setTheme(item.id)}
                            >
                                {item.label}
                            </Button>
                        ))}
                    </Flex>
                </Flex>
            </Card>

            <Card view="outlined" className="yt-card">
                <Flex direction="column" gap={3}>
                    <Text variant="subheader-2">Хранилище</Text>
                    {provider.isLoading ? <Loader size="m" /> : null}
                    {provider.isError ? (
                        <ErrorAlert
                            message={
                                provider.error instanceof Error
                                    ? provider.error.message
                                    : 'Нет данных'
                            }
                        />
                    ) : null}
                    {provider.data ? (
                        <Flex gap={2} wrap>
                            <Label theme="info">{provider.data.provider}</Label>
                            <Label theme={provider.data.ok ? 'success' : 'danger'}>
                                {provider.data.ok ? 'доступно' : 'недоступно'}
                            </Label>
                            {typeof provider.data.usage_bytes === 'number' ? (
                                <Label theme="unknown">{formatBytes(provider.data.usage_bytes)}</Label>
                            ) : null}
                        </Flex>
                    ) : null}
                    {retrieval.data ? (
                        <Text color="secondary" className="yt-mono">
                            Retrieval ({retrieval.data.name}): {retrieval.data.message}
                        </Text>
                    ) : null}
                </Flex>
            </Card>
        </div>
    );
}
