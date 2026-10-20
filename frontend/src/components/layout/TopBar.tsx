import {Moon, Sun} from '@gravity-ui/icons';
import {Button, Flex, Icon, Text, useThemeType} from '@gravity-ui/uikit';
import {useLocation} from 'react-router-dom';

import {useThemeControl} from '@/app/providers';

const titles: Record<string, string> = {
    '/': 'Мои файлы',
    '/shares': 'Общие ссылки',
    '/trash': 'Корзина',
    '/settings': 'Настройки',
};

export function TopBar() {
    const location = useLocation();
    const {theme, setTheme} = useThemeControl();
    const themeType = useThemeType();
    const title = titles[location.pathname] ?? 'A-Tex InfiDrive';

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
                <Button view="flat" size="m" onClick={toggleTheme} title="Переключить тему">
                    <Icon data={themeType === 'dark' ? Sun : Moon} size={16} />
                </Button>
            </Flex>
        </header>
    );
}
