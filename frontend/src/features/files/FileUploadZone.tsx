import {ArrowUpFromLine} from '@gravity-ui/icons';
import {Button, Flex, Icon} from '@gravity-ui/uikit';
import {useCallback, useRef, useState, type DragEvent} from 'react';

interface Props {
    disabled?: boolean;
    onFiles: (files: File[]) => void;
}

export function FileUploadZone({disabled, onFiles}: Props) {
    const inputRef = useRef<HTMLInputElement>(null);
    const [dragOver, setDragOver] = useState(false);

    const take = useCallback(
        (list: FileList | null) => {
            if (!list || list.length === 0) {
                return;
            }
            onFiles(Array.from(list));
        },
        [onFiles],
    );

    const onDrop = (event: DragEvent<HTMLDivElement>) => {
        event.preventDefault();
        setDragOver(false);
        if (disabled) {
            return;
        }
        take(event.dataTransfer.files);
    };

    return (
        <div
            className={dragOver ? 'yt-dropzone yt-dropzone_active' : 'yt-dropzone'}
            onDragOver={(e) => {
                e.preventDefault();
                if (!disabled) {
                    setDragOver(true);
                }
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
        >
            <Flex direction="column" gap={3} alignItems="center">
                <Icon data={ArrowUpFromLine} size={24} />
                <Button
                    view="action"
                    disabled={disabled}
                    onClick={() => inputRef.current?.click()}
                >
                    Выбрать файл
                </Button>
                <input
                    ref={inputRef}
                    type="file"
                    hidden
                    onChange={(e) => {
                        take(e.target.files);
                        e.target.value = '';
                    }}
                />
            </Flex>
        </div>
    );
}
