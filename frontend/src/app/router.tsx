import {BrowserRouter, Navigate, Route, Routes} from 'react-router-dom';

import {AppLayout} from '@/components/layout/AppLayout';
import {LoginPage} from '@/features/auth/LoginPage';
import {RequireAuth} from '@/features/auth/RequireAuth';
import {FilesPage} from '@/features/files/FilesPage';
import {SettingsPage} from '@/features/settings/SettingsPage';
import {SharesPage} from '@/features/shares/SharesPage';
import {TrashPage} from '@/features/trash/TrashPage';

export function AppRouter() {
    return (
        <BrowserRouter>
            <Routes>
                <Route path="/login" element={<LoginPage />} />
                <Route element={<RequireAuth />}>
                    <Route element={<AppLayout />}>
                        <Route index element={<FilesPage />} />
                        <Route path="shares" element={<SharesPage />} />
                        <Route path="trash" element={<TrashPage />} />
                        <Route path="settings" element={<SettingsPage />} />
                        <Route path="*" element={<Navigate to="/" replace />} />
                    </Route>
                </Route>
            </Routes>
        </BrowserRouter>
    );
}
