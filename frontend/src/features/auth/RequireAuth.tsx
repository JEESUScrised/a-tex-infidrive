import {Loader} from '@gravity-ui/uikit';
import {Navigate, Outlet} from 'react-router-dom';

import {useAuthQuery} from '@/hooks/useApiQueries';

export function RequireAuth() {
    const auth = useAuthQuery();

    if (auth.isLoading) {
        return (
            <div className="yt-login">
                <Loader size="l" />
            </div>
        );
    }

    if (auth.isError) {
        return <Navigate to="/login" replace />;
    }

    if (auth.data?.auth_required && !auth.data.authenticated) {
        return <Navigate to="/login" replace />;
    }

    return <Outlet />;
}
