import {Flex} from '@gravity-ui/uikit';
import {Outlet} from 'react-router-dom';

import {Sidebar} from '@/components/layout/Sidebar';
import {TopBar} from '@/components/layout/TopBar';

export function AppLayout() {
    return (
        <div className="yt-shell">
            <Sidebar />
            <Flex direction="column" className="yt-main">
                <TopBar />
                <main className="yt-content">
                    <Outlet />
                </main>
            </Flex>
        </div>
    );
}
