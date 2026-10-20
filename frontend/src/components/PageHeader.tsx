import {Flex, Text} from '@gravity-ui/uikit';
import type {ReactNode} from 'react';

interface Props {
    title: string;
    description?: string;
    actions?: ReactNode;
}

export function PageHeader({title, description, actions}: Props) {
    return (
        <Flex justifyContent="space-between" alignItems="flex-start" gap={4} className="yt-page-header">
            <Flex direction="column" gap={1}>
                <Text variant="header-1">{title}</Text>
                {description ? <Text color="secondary">{description}</Text> : null}
            </Flex>
            {actions ? <Flex gap={2}>{actions}</Flex> : null}
        </Flex>
    );
}
