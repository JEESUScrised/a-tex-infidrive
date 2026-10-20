import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';

import {AppLayout} from '@/components/layout/AppLayout';
import {LoginPage} from '@/features/auth/LoginPage';
import {RequireAuth} from '@/features/auth/RequireAuth';
import {FilesPage} from '@/features/files/FilesPage';
import {JobsPage} from '@/features/jobs/JobsPage';
import {RecoveryPage} from '@/features/recovery/RecoveryPage';
import {SettingsPage} from '@/features/settings/SettingsPage';
import {YouTubePage} from '@/features/youtube/YouTubePage';

export function AppRouter() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path="/login" element={<LoginPage />} />
                <Route element={<RequireAuth />}>
                    <Route element={<AppLayout />}>
                        <Route index element={<FilesPage />} />
                        <Route path="jobs" element={<JobsPage />} />
                        <Route path="recovery" element={<RecoveryPage />} />
                        <Route path="youtube" element={<YouTubePage />} />
                        <Route path="settings" element={<SettingsPage />} />
                        <Route path="*" element={<Navigate to="/" replace />} />
                    </Route>
                </Route>
            </Routes>
        </BrowserRouter>
    );
}
