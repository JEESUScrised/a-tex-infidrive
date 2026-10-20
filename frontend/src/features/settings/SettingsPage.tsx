import {Button, Card, Flex, Label, Loader, Text, useToaster} from '@gravity-ui/uikit';
import type {Theme} from '@gravity-ui/uikit';
import {useEffect} from 'react';
import {useSearchParams} from 'react-router-dom';

import {useThemeControl} from '@/app/providers';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {api} from '@/api/endpoints';
import {
    useAuthQuery,
    useLogoutMutation,
    useProviderQuery,
    useYouTubeConnectMutation,
    useYouTubeDisconnectMutation,
    useYouTubeQuery,
} from '@/hooks/useApiQueries';
import {queryKeys} from '@/hooks/queryKeys';
import {formatBytes} from '@/lib/format';
import {ApiError} from '@/types/api';
import {useQuery, useQueryClient} from '@tanstack/react-query';

const themes: {id: Theme; label: string}[] = [
    {id: 'system', label: 'Системная'},
    {id: 'light', label: 'Светлая'},
    {id: 'dark', label: 'Тёмная'},
];

export function SettingsPage() {
    const auth = useAuthQuery();
    const logout = useLogoutMutation();
    const provider = useProviderQuery();
    const youtube = useYouTubeQuery();
    const connect = useYouTubeConnectMutation();
    const disconnect = useYouTubeDisconnectMutation();
    const retrieval = useQuery({
        queryKey: queryKeys.retrieval,
        queryFn: api.retrievalStatus,
        retry: false,
    });
    const {theme, setTheme} = useThemeControl();
    const toaster = useToaster();
    const [params, setParams] = useSearchParams();
    const qc = useQueryClient();

    const isAdmin = Boolean(auth.data && (!auth.data.auth_required || auth.data.authenticated));

    useEffect(() => {
        const connected = params.get('youtube');
        const err = params.get('youtube_error');
        if (!connected && !err) {
            return;
        }
        if (connected === 'connected') {
            toaster.add({name: 'yt-ok', title: 'YouTube подключён', theme: 'success'});
            void qc.invalidateQueries({queryKey: queryKeys.youtube});
        }
        if (err) {
            toaster.add({name: 'yt-err', title: 'Ошибка OAuth', content: err, theme: 'danger'});
        }
        const next = new URLSearchParams(params);
        next.delete('youtube');
        next.delete('youtube_error');
        setParams(next, {replace: true});
    }, [params, setParams, toaster, qc]);

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

            {isAdmin ? (
                <Card view="outlined" className="yt-card">
                    <Flex direction="column" gap={3}>
                        <Text variant="subheader-2">YouTube Storage</Text>
                        {youtube.isLoading ? <Loader size="m" /> : null}
                        {youtube.isError ? (
                            <ErrorAlert
                                message={
                                    youtube.error instanceof ApiError
                                        ? youtube.error.message
                                        : youtube.error instanceof Error
                                          ? youtube.error.message
                                          : 'Ошибка статуса YouTube'
                                }
                            />
                        ) : null}
                        {youtube.data ? (
                            <>
                                <Flex gap={2} wrap>
                                    <Label theme={youtube.data.oauth_credentials_set ? 'success' : 'danger'}>
                                        {youtube.data.oauth_credentials_set
                                            ? 'OAuth credentials заданы'
                                            : 'OAuth credentials отсутствуют'}
                                    </Label>
                                    <Label theme={youtube.data.enabled ? 'success' : 'warning'}>
                                        {youtube.data.enabled ? 'API настроен' : 'API не настроен'}
                                    </Label>
                                    <Label theme={youtube.data.connected ? 'success' : 'unknown'}>
                                        {youtube.data.connected ? 'аккаунт подключён' : 'аккаунт не подключён'}
                                    </Label>
                                    {youtube.data.privacy_status ? (
                                        <Label theme="info">privacy: {youtube.data.privacy_status}</Label>
                                    ) : null}
                                    {youtube.data.experimental_store ? (
                                        <Label theme="info">experimental store</Label>
                                    ) : null}
                                </Flex>
                                <Text color="secondary">{youtube.data.message}</Text>
                                {youtube.data.channel ? (
                                    <Text>
                                        Канал: {youtube.data.channel.title}
                                        <Text color="secondary" className="yt-mono">
                                            {' '}
                                            ({youtube.data.channel.id})
                                        </Text>
                                    </Text>
                                ) : null}
                                {youtube.data.redirect_uri ? (
                                    <Text color="secondary" className="yt-mono">
                                        Redirect URI: {youtube.data.redirect_uri}
                                    </Text>
                                ) : null}
                                {youtube.data.retrieval ? (
                                    <Text color="secondary">
                                        Retrieval ({youtube.data.retrieval.name}):{' '}
                                        {youtube.data.retrieval.available ? 'доступен' : 'недоступен'} —{' '}
                                        {youtube.data.retrieval.message}
                                    </Text>
                                ) : null}
                                {!youtube.data.oauth_credentials_set ? (
                                    <ErrorAlert message="Задайте GOOGLE_CLIENT_ID и GOOGLE_CLIENT_SECRET в /etc/infidrive/env, затем перезапустите сервис." />
                                ) : null}
                                <Flex gap={2} wrap>
                                    {youtube.data.enabled && !youtube.data.connected ? (
                                        <Button
                                            view="action"
                                            loading={connect.isPending}
                                            onClick={() => connect.mutate()}
                                        >
                                            Подключить Google
                                        </Button>
                                    ) : null}
                                    {youtube.data.enabled && youtube.data.connected ? (
                                        <>
                                            <Button
                                                view="outlined"
                                                loading={connect.isPending}
                                                onClick={() => connect.mutate()}
                                            >
                                                Переподключить
                                            </Button>
                                            <Button
                                                view="outlined-danger"
                                                loading={disconnect.isPending}
                                                onClick={() => disconnect.mutate()}
                                            >
                                                Отключить аккаунт
                                            </Button>
                                        </>
                                    ) : null}
                                </Flex>
                                {connect.isError ? (
                                    <ErrorAlert
                                        message={
                                            connect.error instanceof Error
                                                ? connect.error.message
                                                : 'Не удалось начать OAuth'
                                        }
                                    />
                                ) : null}
                                {disconnect.isError ? (
                                    <ErrorAlert
                                        message={
                                            disconnect.error instanceof Error
                                                ? disconnect.error.message
                                                : 'Не удалось отключить'
                                        }
                                    />
                                ) : null}
                            </>
                        ) : null}
                    </Flex>
                </Card>
            ) : null}
        </div>
    );
}
