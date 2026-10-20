import {Flex} from '@gravity-ui/uikit';

import {PageTransition} from '@/components/layout/PageTransition';
import {Sidebar} from '@/components/layout/Sidebar';
import {TopBar} from '@/components/layout/TopBar';

export function AppLayout() {
    return (
        <div className="yt-shell">
            <Sidebar />
            <Flex direction="column" className="yt-main">
                <TopBar />
                <main className="yt-content">
                    <PageTransition />
                </main>
            </Flex>
        </div>
    );
}
