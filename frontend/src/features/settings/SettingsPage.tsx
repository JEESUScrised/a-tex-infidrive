import {Button, Card, Flex, Label, Loader, Text} from '@gravity-ui/uikit';
import type {Theme} from '@gravity-ui/uikit';

import {useThemeControl} from '@/app/providers';
import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {useAuthQuery, useLogoutMutation, useSystemQuery} from '@/hooks/useApiQueries';

const themes: {id: Theme; label: string}[] = [
    {id: 'system', label: 'Системная'},
    {id: 'light', label: 'Светлая'},
    {id: 'dark', label: 'Тёмная'},
];

export function SettingsPage() {
    const system = useSystemQuery();
    const auth = useAuthQuery();
    const logout = useLogoutMutation();
    const {theme, setTheme} = useThemeControl();

    return (
        <div className="yt-page">
            <PageHeader
                title="Настройки"
                description="Локальные параметры интерфейса и диагностика сервера. Секреты OAuth здесь не отображаются."
            />

            <Card view="outlined" className="yt-card">
                <Flex direction="column" gap={3}>
                    <Text variant="subheader-2">Сессия</Text>
                    <Text color="secondary">
                        {auth.data?.username
                            ? `Администратор: ${auth.data.username}`
                            : 'Локальный режим без обязательного входа'}
                    </Text>
                    {auth.data?.auth_required ? (
                        <Button
                            view="outlined"
                            onClick={() => logout.mutate()}
                            loading={logout.isPending}
                        >
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
                    <Text variant="subheader-2">Сервер</Text>
                    {system.isLoading ? <Loader size="m" /> : null}
                    {system.isError ? (
                        <ErrorAlert
                            message={
                                system.error instanceof Error
                                    ? system.error.message
                                    : 'Нет связи с backend'
                            }
                        />
                    ) : null}
                    {system.data ? (
                        <>
                            <Flex gap={2} wrap>
                                <Label theme="success">v{system.data.version}</Label>
                                {typeof system.data.ffmpeg === 'boolean' ? (
                                    <Label theme={system.data.ffmpeg ? 'success' : 'danger'}>
                                        FFmpeg {system.data.ffmpeg ? 'найден' : 'не найден'}
                                    </Label>
                                ) : null}
                                {system.data.ffmpeg_source ? (
                                    <Label theme="info">{system.data.ffmpeg_source}</Label>
                                ) : null}
                            </Flex>
                            {system.data.ffmpeg_info ? (
                                <Text className="yt-mono" color="secondary">
                                    {system.data.ffmpeg_info}
                                </Text>
                            ) : null}
                        </>
                    ) : null}
                </Flex>
            </Card>
        </div>
    );
}
