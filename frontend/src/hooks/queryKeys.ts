export const queryKeys = {
    health: ['health'] as const,
    system: ['system'] as const,
    auth: ['auth'] as const,
    drive: (folderId: string) => ['drive', folderId] as const,
    trash: ['trash'] as const,
    shares: ['shares'] as const,
    provider: ['provider'] as const,
};
