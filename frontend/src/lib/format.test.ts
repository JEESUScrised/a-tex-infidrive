import {describe, expect, it} from 'vitest';

import {canCancelJob, formatBytes, jobKindLabel, shortHash} from '@/lib/format';

describe('format helpers', () => {
    it('formats bytes', () => {
        expect(formatBytes(500)).toBe('500 Б');
        expect(formatBytes(2048)).toContain('КБ');
    });

    it('shortens hash', () => {
        const hash = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
        expect(shortHash(hash)).toContain('…');
    });

    it('labels job kinds', () => {
        expect(jobKindLabel('youtube_upload')).toBe('YouTube');
        expect(canCancelJob('uploading')).toBe(true);
        expect(canCancelJob('completed')).toBe(false);
    });
});
