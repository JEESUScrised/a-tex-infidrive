import {Flex, Text} from '@gravity-ui/uikit';
import type {ReactNode} from 'react';

interface Props {
    title: string;
    description?: string;
    action?: ReactNode;
}

export function EmptyState({title, description, action}: Props) {
    return (
        <Flex direction="column" gap={3} alignItems="center" className="yt-empty">
            <Text variant="subheader-2">{title}</Text>
            {description ? (
                <Text color="secondary" style={{textAlign: 'center', maxWidth: 420}}>
                    {description}
                </Text>
            ) : null}
            {action}
        </Flex>
    );
}
