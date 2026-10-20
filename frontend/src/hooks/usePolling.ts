import type {Job} from '@/types/api';

const ACTIVE: ReadonlySet<string> = new Set([
    'queued',
    'running',
    'uploading',
    'uploaded',
    'processing',
]);

export function hasActiveJobs(jobs: Job[] | undefined): boolean {
    return Boolean(jobs?.some((job) => ACTIVE.has(job.status)));
}

/** Единый интервал опроса: чаще, пока есть активные задания. */
export function jobsRefetchInterval(jobs: Job[] | undefined): number | false {
    return hasActiveJobs(jobs) ? 2000 : 8000;
}
