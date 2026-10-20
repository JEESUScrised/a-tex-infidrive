import {Moon, Sun} from '@gravity-ui/icons';
import {Button, Flex, Icon, Label, Text, useThemeType} from '@gravity-ui/uikit';
import {useMemo} from 'react';
import {useLocation} from 'react-router-dom';

import {useThemeControl} from '@/app/providers';
import {useYouTubeQuery} from '@/hooks/useApiQueries';

const titles: Record<string, string> = {
    '/': 'Файлы',
    '/jobs': 'Задания',
    '/recovery': 'Восстановление',
    '/youtube': 'YouTube',
    '/settings': 'Настройки',
};

export function TopBar() {
    const location = useLocation();
    const {theme, setTheme} = useThemeControl();
    const themeType = useThemeType();
    const youtube = useYouTubeQuery();

    const title = titles[location.pathname] ?? 'YTDrive';

    const ytLabel = useMemo(() => {
        if (!youtube.data) {
            return {theme: 'unknown' as const, text: 'YouTube…'};
        }
        if (!youtube.data.enabled) {
            return {theme: 'unknown' as const, text: 'YouTube не настроен'};
        }
        if (youtube.data.connected) {
            const name = youtube.data.channel?.title;
            return {
                theme: 'success' as const,
                text: name ? `YouTube: ${name}` : 'YouTube подключён',
            };
        }
        return {theme: 'warning' as const, text: 'YouTube: не подключён'};
    }, [youtube.data]);

    const toggleTheme = () => {
        if (theme === 'system') {
            setTheme(themeType === 'dark' ? 'light' : 'dark');
            return;
        }
        setTheme(theme === 'dark' ? 'light' : 'dark');
    };

    return (
        <header className="yt-topbar">
            <Text variant="subheader-2">{title}</Text>
            <Flex gap={3} alignItems="center">
                <Label theme={ytLabel.theme} size="s">
                    {ytLabel.text}
                </Label>
                <Button view="flat" size="m" onClick={toggleTheme} title="Переключить тему">
                    <Icon data={themeType === 'dark' ? Sun : Moon} size={16} />
                </Button>
            </Flex>
        </header>
    );
}
