export const queryKeys = {
    health: ['health'] as const,
    files: ['files'] as const,
    file: (id: string) => ['files', id] as const,
    jobs: ['jobs'] as const,
    job: (id: string) => ['jobs', id] as const,
    youtube: ['youtube'] as const,
};
