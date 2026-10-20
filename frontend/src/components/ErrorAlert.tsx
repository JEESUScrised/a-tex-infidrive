import {Alert} from '@gravity-ui/uikit';

interface Props {
    title?: string;
    message: string | null | undefined;
}

export function ErrorAlert({title = 'Ошибка', message}: Props) {
    if (!message) {
        return null;
    }
    return <Alert theme="danger" title={title} message={message} />;
}
