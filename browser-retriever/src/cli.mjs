#!/usr/bin/env node
/**
 * CLI for Go adapter:
 *   node src/cli.mjs --video-id=ID --out-dir=DIR [--timeout-ms=120000] [--headed]
 *
 * Progress/logs → stderr (JSON lines).
 * Final result → stdout (single JSON object).
 */
import fs from 'node:fs';
import {retrieveMP4} from './retrieve.mjs';

function parseArgs(argv) {
  const out = {videoID: '', outDir: '', timeoutMs: 120_000, headless: true};
  for (const a of argv) {
    if (a.startsWith('--video-id=')) out.videoID = a.slice('--video-id='.length);
    else if (a.startsWith('--out-dir=')) out.outDir = a.slice('--out-dir='.length);
    else if (a.startsWith('--timeout-ms=')) out.timeoutMs = Number(a.slice('--timeout-ms='.length)) || out.timeoutMs;
    else if (a === '--headed') out.headless = false;
    else if (a === '--help' || a === '-h') out.help = true;
  }
  return out;
}

const args = parseArgs(process.argv.slice(2));
if (args.help || !args.videoID || !args.outDir) {
  console.error('Usage: node src/cli.mjs --video-id=ID --out-dir=DIR [--timeout-ms=N] [--headed]');
  process.exit(args.help ? 0 : 2);
}

fs.mkdirSync(args.outDir, {recursive: true});

const result = await retrieveMP4({
  videoID: args.videoID,
  outDir: args.outDir,
  timeoutMs: args.timeoutMs,
  headless: args.headless,
});

process.stdout.write(JSON.stringify(result) + '\n');
process.exit(result.ok ? 0 : 1);
