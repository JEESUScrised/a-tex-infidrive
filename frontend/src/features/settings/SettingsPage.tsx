import {Button, Card, Flex, Label, Loader, Text} from '@gravity-ui/uikit';

import {ErrorAlert} from '@/components/ErrorAlert';
import {PageHeader} from '@/components/PageHeader';
import {useThemeControl} from '@/app/providers';
import {useHealthQuery} from '@/hooks/useApiQueries';
import type {Theme} from '@gravity-ui/uikit';

const themes: {id: Theme; label: string}[] = [
    {id: 'system', label: 'Системная'},
    {id: 'light', label: 'Светлая'},
    {id: 'dark', label: 'Тёмная'},
];

export function SettingsPage() {
    const health = useHealthQuery();
    const {theme, setTheme} = useThemeControl();

    return (
        <div className="yt-page">
            <PageHeader
                title="Настройки"
                description="Локальные параметры интерфейса и диагностика сервера. Секреты OAuth здесь не отображаются."
            />

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
                    {health.isLoading ? <Loader size="m" /> : null}
                    {health.isError ? (
                        <ErrorAlert
                            message={
                                health.error instanceof Error
                                    ? health.error.message
                                    : 'Нет связи с backend'
                            }
                        />
                    ) : null}
                    {health.data ? (
                        <>
                            <Flex gap={2} wrap>
                                <Label theme="success">v{health.data.version}</Label>
                                <Label theme={health.data.ffmpeg ? 'success' : 'danger'}>
                                    FFmpeg {health.data.ffmpeg ? 'найден' : 'не найден'}
                                </Label>
                                <Label theme="info">{health.data.ffmpeg_source}</Label>
                            </Flex>
                            <Text className="yt-mono" color="secondary">
                                {health.data.ffmpeg_info}
                            </Text>
                        </>
                    ) : null}
                </Flex>
            </Card>
        </div>
    );
}
