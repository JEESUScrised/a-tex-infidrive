import {ThemeProvider, Toaster, ToasterComponent, ToasterProvider} from '@gravity-ui/uikit';
import type {Theme} from '@gravity-ui/uikit';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {createContext, useContext, useMemo, useState, type ReactNode} from 'react';

const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            retry: 1,
            refetchOnWindowFocus: false,
        },
    },
});

const toaster = new Toaster();

interface ThemeControl {
    theme: Theme;
    setTheme: (theme: Theme) => void;
}

const ThemeControlContext = createContext<ThemeControl | null>(null);

export function useThemeControl(): ThemeControl {
    const ctx = useContext(ThemeControlContext);
    if (!ctx) {
        throw new Error('useThemeControl вне ThemeControlContext');
    }
    return ctx;
}

export function AppProviders({children}: {children: ReactNode}) {
    const [theme, setTheme] = useState<Theme>(() => {
        const saved = localStorage.getItem('ytdrive-theme');
        if (saved === 'light' || saved === 'dark' || saved === 'system') {
            return saved;
        }
        return 'system';
    });

    const control = useMemo(
        () => ({
            theme,
            setTheme: (next: Theme) => {
                localStorage.setItem('ytdrive-theme', next);
                setTheme(next);
            },
        }),
        [theme],
    );

    return (
        <QueryClientProvider client={queryClient}>
            <ThemeControlContext.Provider value={control}>
                <ThemeProvider theme={theme}>
                    <ToasterProvider toaster={toaster}>
                        {children}
                        <ToasterComponent />
                    </ToasterProvider>
                </ThemeProvider>
            </ThemeControlContext.Provider>
        </QueryClientProvider>
    );
}
