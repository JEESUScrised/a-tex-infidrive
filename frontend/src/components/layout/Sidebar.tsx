import {ArrowRotateLeft, Cloud, FolderOpen, Gear, ListCheck} from '@gravity-ui/icons';
import {Flex, Icon, Text} from '@gravity-ui/uikit';
import {NavLink} from 'react-router-dom';

const items = [
    {to: '/', end: true, label: 'Файлы', icon: FolderOpen},
    {to: '/jobs', label: 'Задания', icon: ListCheck},
    {to: '/recovery', label: 'Восстановление', icon: ArrowRotateLeft},
    {to: '/youtube', label: 'YouTube', icon: Cloud},
    {to: '/settings', label: 'Настройки', icon: Gear},
] as const;

export function Sidebar() {
    return (
        <aside className="yt-sidebar">
            <Flex direction="column" gap={1} className="yt-brand">
                <Text variant="header-2">YTDrive</Text>
                <Text color="secondary" variant="caption-2">
                    файл ↔ MP4 ↔ YouTube
                </Text>
            </Flex>
            <nav className="yt-nav">
                {items.map((item) => (
                    <NavLink
                        key={item.to}
                        to={item.to}
                        end={'end' in item ? item.end : false}
                        className={({isActive}) =>
                            isActive ? 'yt-nav__link yt-nav__link_active' : 'yt-nav__link'
                        }
                    >
                        <Icon data={item.icon} size={16} />
                        <span>{item.label}</span>
                    </NavLink>
                ))}
            </nav>
        </aside>
    );
}
