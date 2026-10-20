import {Alert, Button, Card, Flex, Text, TextInput} from '@gravity-ui/uikit';
import {useState, type FormEvent} from 'react';
import {Navigate} from 'react-router-dom';

import {useAuthQuery, useLoginMutation} from '@/hooks/useApiQueries';
import {ApiError} from '@/types/api';

export function LoginPage() {
    const auth = useAuthQuery();
    const login = useLoginMutation();
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');

    if (auth.isLoading) {
        return (
            <div className="yt-login">
                <Text color="secondary">Проверка сессии…</Text>
            </div>
        );
    }

    if (auth.data && (!auth.data.auth_required || auth.data.authenticated)) {
        return <Navigate to="/" replace />;
    }

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        login.mutate({username, password});
    };

    const error = login.isError
        ? login.error instanceof ApiError
            ? login.error.message
            : login.error instanceof Error
              ? login.error.message
              : 'Не удалось войти'
        : null;

    return (
        <div className="yt-login">
            <Card view="outlined" className="yt-login__card">
                <form onSubmit={onSubmit}>
                    <Flex direction="column" gap={4}>
                        <div>
                            <Text variant="header-1">A-Tex InfiDrive</Text>
                            <Text color="secondary">Вход администратора</Text>
                        </div>
                        {error ? <Alert theme="danger" title={error} /> : null}
                        <TextInput
                            label="Логин"
                            value={username}
                            onUpdate={setUsername}
                            autoComplete="username"
                            size="l"
                        />
                        <TextInput
                            label="Пароль"
                            type="password"
                            value={password}
                            onUpdate={setPassword}
                            autoComplete="current-password"
                            size="l"
                        />
                        <Button view="action" size="l" type="submit" loading={login.isPending}>
                            Войти
                        </Button>
                    </Flex>
                </form>
            </Card>
        </div>
    );
}
