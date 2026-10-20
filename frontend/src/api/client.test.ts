import {afterEach, describe, expect, it, vi} from 'vitest';

import {apiRequest} from '@/api/client';
import {ApiError} from '@/types/api';

describe('apiRequest', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('returns JSON on success', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue(
                new Response(JSON.stringify({ok: true}), {
                    status: 200,
                    headers: {'Content-Type': 'application/json'},
                }),
            ),
        );
        const data = await apiRequest<{ok: boolean}>('/api/health');
        expect(data.ok).toBe(true);
    });

    it('throws ApiError with message from body', async () => {
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue(
                new Response(JSON.stringify({error: 'очередь заполнена'}), {
                    status: 429,
                    headers: {'Content-Type': 'application/json'},
                }),
            ),
        );
        try {
            await apiRequest('/api/files');
            expect.fail('expected error');
        } catch (err) {
            expect(err).toBeInstanceOf(ApiError);
            expect((err as ApiError).message).toBe('очередь заполнена');
            expect((err as ApiError).status).toBe(429);
        }
    });
});
