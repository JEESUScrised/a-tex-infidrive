import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';

import {api} from '@/api/endpoints';
import {queryKeys} from '@/hooks/queryKeys';
import {hasActiveJobs, jobsRefetchInterval} from '@/hooks/usePolling';

export function useHealthQuery() {
    return useQuery({
        queryKey: queryKeys.health,
        queryFn: api.health,
        staleTime: 30_000,
    });
}

export function useJobsQuery() {
    return useQuery({
        queryKey: queryKeys.jobs,
        queryFn: api.listJobs,
        refetchInterval: (query) => jobsRefetchInterval(query.state.data),
    });
}

export function useFilesQuery() {
    const jobsQuery = useJobsQuery();
    return useQuery({
        queryKey: queryKeys.files,
        queryFn: api.listFiles,
        refetchInterval: hasActiveJobs(jobsQuery.data) ? 2000 : 10_000,
    });
}

export function useYouTubeQuery() {
    return useQuery({
        queryKey: queryKeys.youtube,
        queryFn: api.youtubeStatus,
        staleTime: 15_000,
    });
}

export function useInvalidateAll() {
    const qc = useQueryClient();
    return async () => {
        await Promise.all([
            qc.invalidateQueries({queryKey: queryKeys.files}),
            qc.invalidateQueries({queryKey: queryKeys.jobs}),
            qc.invalidateQueries({queryKey: queryKeys.youtube}),
            qc.invalidateQueries({queryKey: queryKeys.health}),
        ]);
    };
}

export function useUploadMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.uploadFile,
        onSuccess: () => invalidate(),
    });
}

export function useEncodeMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.encodeFile,
        onSuccess: () => invalidate(),
    });
}

export function useYouTubeUploadMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.uploadToYouTube,
        onSuccess: () => invalidate(),
    });
}

export function useRecoverMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.recoverFromMp4,
        onSuccess: () => invalidate(),
    });
}

export function useCancelJobMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.cancelJob,
        onSuccess: () => invalidate(),
    });
}

export function useYouTubeDisconnectMutation() {
    const invalidate = useInvalidateAll();
    return useMutation({
        mutationFn: api.youtubeDisconnect,
        onSuccess: () => invalidate(),
    });
}

export function useYouTubeAuthMutation() {
    return useMutation({
        mutationFn: api.youtubeAuthStart,
        onSuccess: (data) => {
            window.location.assign(data.auth_url);
        },
    });
}
