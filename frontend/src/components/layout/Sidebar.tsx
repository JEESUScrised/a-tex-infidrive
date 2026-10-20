import {FolderOpen, Gear, Link, TrashBin} from '@gravity-ui/icons';
import {Icon, Text} from '@gravity-ui/uikit';
import {NavLink} from 'react-router-dom';

const items = [
    {to: '/', end: true, label: 'Мои файлы', icon: FolderOpen},
    {to: '/shares', label: 'Общие ссылки', icon: Link},
    {to: '/trash', label: 'Корзина', icon: TrashBin},
    {to: '/settings', label: 'Настройки', icon: Gear},
] as const;

export function Sidebar() {
    return (
        <aside className="yt-sidebar">
            <div className="yt-brand">
                <Text variant="header-2">InfiDrive</Text>
            </div>
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
