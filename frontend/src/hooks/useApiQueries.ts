import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';

import {api} from '@/api/endpoints';
import {queryKeys} from '@/hooks/queryKeys';

export function useHealthQuery() {
    return useQuery({
        queryKey: queryKeys.health,
        queryFn: api.health,
        staleTime: 30_000,
    });
}

export function useSystemQuery() {
    return useQuery({
        queryKey: queryKeys.system,
        queryFn: api.system,
        staleTime: 30_000,
    });
}

export function useAuthQuery() {
    return useQuery({
        queryKey: queryKeys.auth,
        queryFn: api.authMe,
        staleTime: 60_000,
        retry: false,
    });
}

export function useDriveQuery(folderId: string) {
    return useQuery({
        queryKey: queryKeys.drive(folderId),
        queryFn: () => api.listDrive(folderId),
    });
}

export function useTrashQuery() {
    return useQuery({
        queryKey: queryKeys.trash,
        queryFn: api.listTrash,
    });
}

export function useSharesQuery() {
    return useQuery({
        queryKey: queryKeys.shares,
        queryFn: api.listShares,
    });
}

export function useProviderQuery() {
    return useQuery({
        queryKey: queryKeys.provider,
        queryFn: api.providerStatus,
        staleTime: 15_000,
    });
}

export function useInvalidateDrive() {
    const qc = useQueryClient();
    return async () => {
        await Promise.all([
            qc.invalidateQueries({queryKey: ['drive']}),
            qc.invalidateQueries({queryKey: queryKeys.trash}),
            qc.invalidateQueries({queryKey: queryKeys.shares}),
            qc.invalidateQueries({queryKey: queryKeys.provider}),
        ]);
    };
}

export function useLoginMutation() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({username, password}: {username: string; password: string}) =>
            api.login(username, password),
        onSuccess: async () => {
            await qc.invalidateQueries({queryKey: queryKeys.auth});
        },
    });
}

export function useLogoutMutation() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: api.logout,
        onSuccess: async () => {
            qc.clear();
            await qc.invalidateQueries({queryKey: queryKeys.auth});
        },
    });
}

export function useYouTubeQuery() {
    return useQuery({
        queryKey: queryKeys.youtube,
        queryFn: api.youtubeStatus,
        staleTime: 15_000,
        retry: false,
    });
}

export function useYouTubeConnectMutation() {
    return useMutation({
        mutationFn: api.youtubeAuthStart,
        onSuccess: ({auth_url}) => {
            window.location.assign(auth_url);
        },
    });
}

export function useYouTubeDisconnectMutation() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: api.youtubeDisconnect,
        onSuccess: async () => {
            await qc.invalidateQueries({queryKey: queryKeys.youtube});
        },
    });
}

export function useUploadMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: ({file, folderId}: {file: File; folderId: string}) =>
            api.uploadFile(file, folderId),
        onSuccess: () => invalidate(),
    });
}

export function useCreateFolderMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: ({name, parentId}: {name: string; parentId: string}) =>
            api.createFolder(name, parentId),
        onSuccess: () => invalidate(),
    });
}

export function useTrashMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: api.trashFile,
        onSuccess: () => invalidate(),
    });
}

export function useRestoreMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: api.restoreFile,
        onSuccess: () => invalidate(),
    });
}

export function usePurgeMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: api.purgeFile,
        onSuccess: () => invalidate(),
    });
}

export function useShareMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: ({id, ttlHours, maxDownloads}: {id: string; ttlHours?: number; maxDownloads?: number}) =>
            api.createShare(id, ttlHours ?? 0, maxDownloads ?? 0),
        onSuccess: () => invalidate(),
    });
}

export function useRevokeShareMutation() {
    const invalidate = useInvalidateDrive();
    return useMutation({
        mutationFn: api.revokeShare,
        onSuccess: () => invalidate(),
    });
}
