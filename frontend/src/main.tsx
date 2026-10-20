import {StrictMode} from 'react';
import {createRoot} from 'react-dom/client';

import '@gravity-ui/uikit/styles/fonts.css';
import '@gravity-ui/uikit/styles/styles.css';

import {App} from '@/app/App';
import {AppProviders} from '@/app/providers';
import '@/styles/app.scss';

const root = document.getElementById('root');
if (!root) {
    throw new Error('root element not found');
}

createRoot(root).render(
    <StrictMode>
        <AppProviders>
            <App />
        </AppProviders>
    </StrictMode>,
);
