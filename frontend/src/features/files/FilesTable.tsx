import {
    ArrowDownToLine,
    ArrowUpFromLine,
    CloudArrowUpIn,
    Filmstrip,
} from '@gravity-ui/icons';
import {Button, Flex, Icon, Label, Table, Text} from '@gravity-ui/uikit';
import type {TableColumnConfig} from '@gravity-ui/uikit';

import {fileUrls} from '@/api/endpoints';
import {fileStatusLabel, formatBytes, formatDate, shortHash} from '@/lib/format';
import type {StoredFile} from '@/types/api';

interface Props {
    files: StoredFile[];
    youtubeReady: boolean;
    busyId: string | null;
    onEncode: (id: string) => void;
    onYouTube: (id: string) => void;
}

export function FilesTable({files, youtubeReady, busyId, onEncode, onYouTube}: Props) {
    const columns: TableColumnConfig<StoredFile>[] = [
        {
            id: 'original_name',
            name: 'Имя',
            primary: true,
            width: 220,
            template: (item) => (
                <Text className="yt-ellipsis" title={item.original_name}>
                    {item.original_name}
                </Text>
            ),
        },
        {
            id: 'size',
            name: 'Размер',
            width: 100,
            template: (item) => formatBytes(item.size),
        },
        {
            id: 'created_at',
            name: 'Добавлен',
            width: 150,
            template: (item) => formatDate(item.created_at),
        },
        {
            id: 'status',
            name: 'Статус',
            width: 130,
            template: (item) => {
                const st = fileStatusLabel(item);
                return (
                    <Label theme={st.theme} size="xs">
                        {st.text}
                    </Label>
                );
            },
        },
        {
            id: 'sha256',
            name: 'SHA-256',
            width: 140,
            template: (item) => (
                <Text className="yt-mono" title={item.sha256}>
                    {shortHash(item.sha256)}
                </Text>
            ),
        },
        {
            id: 'actions',
            name: 'Действия',
            width: 340,
            template: (item) => (
                <Flex gap={1} wrap="wrap">
                    <Button
                        size="s"
                        view="outlined"
                        loading={busyId === item.id}
                        onClick={() => onEncode(item.id)}
                    >
                        <Icon data={Filmstrip} size={14} />
                        Кодировать
                    </Button>
                    <Button size="s" view="flat" href={fileUrls.original(item.id)}>
                        <Icon data={ArrowDownToLine} size={14} />
                        Файл
                    </Button>
                    {item.has_video ? (
                        <Button size="s" view="flat" href={fileUrls.video(item.id)}>
                            <Icon data={ArrowUpFromLine} size={14} />
                            MP4
                        </Button>
                    ) : null}
                    {item.has_video && youtubeReady && !item.youtube_video_id ? (
                        <Button
                            size="s"
                            view="outlined-success"
                            loading={busyId === item.id}
                            onClick={() => onYouTube(item.id)}
                        >
                            <Icon data={CloudArrowUpIn} size={14} />
                            YouTube
                        </Button>
                    ) : null}
                    {item.youtube_url ? (
                        <Button size="s" view="flat-info" href={item.youtube_url} target="_blank">
                            Открыть
                        </Button>
                    ) : null}
                </Flex>
            ),
        },
    ];

    return (
        <Table
            className="yt-table"
            data={files}
            columns={columns}
            getRowId={(item) => item.id}
            wordWrap
            width="max"
        />
    );
}
