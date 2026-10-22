#!/usr/bin/env node
/**
 * YouTube Studio owner-download CLI (PoC).
 *
 * Interactive login (headed, manual Google/2FA):
 *   node src/studio-cli.mjs --login --profile-dir=./.studio-profile
 *
 * Download own video:
 *   node src/studio-cli.mjs --video-id=ID --out-dir=DIR --profile-dir=./.studio-profile [--headed]
 *
 * Progress → stderr JSON lines. Final result → stdout JSON.
 */
import path from 'node:path';
import {loginStudio, retrieveStudioMP4} from './studio-retrieve.mjs';

function parseArgs(argv) {
  const out = {
    login: false,
    videoID: '',
    outDir: '',
    profileDir: '',
    timeoutMs: 180_000,
    headless: true,
    help: false,
  };
  for (const a of argv) {
    if (a === '--login') out.login = true;
    else if (a.startsWith('--video-id=')) out.videoID = a.slice('--video-id='.length);
    else if (a.startsWith('--out-dir=')) out.outDir = a.slice('--out-dir='.length);
    else if (a.startsWith('--profile-dir=')) out.profileDir = a.slice('--profile-dir='.length);
    else if (a.startsWith('--timeout-ms=')) out.timeoutMs = Number(a.slice('--timeout-ms='.length)) || out.timeoutMs;
    else if (a === '--headed') out.headless = false;
    else if (a === '--help' || a === '-h') out.help = true;
  }
  if (!out.profileDir) {
    out.profileDir = path.resolve('browser-retriever/.studio-profile');
  } else {
    out.profileDir = path.resolve(out.profileDir);
  }
  return out;
}

const args = parseArgs(process.argv.slice(2));
if (args.help) {
  console.error(`Usage:
  node src/studio-cli.mjs --login [--profile-dir=DIR]
  node src/studio-cli.mjs --video-id=ID --out-dir=DIR [--profile-dir=DIR] [--headed] [--timeout-ms=N]`);
  process.exit(0);
}

if (args.login) {
  const result = await loginStudio({
    profileDir: args.profileDir,
    timeoutMs: Math.max(args.timeoutMs, 10 * 60_000),
  });
  process.stdout.write(JSON.stringify(result) + '\n');
  process.exit(result.ok ? 0 : 1);
}

if (!args.videoID || !args.outDir) {
  console.error('Need --video-id and --out-dir (or --login). See --help.');
  process.exit(2);
}

const result = await retrieveStudioMP4({
  videoID: args.videoID,
  outDir: path.resolve(args.outDir),
  profileDir: args.profileDir,
  timeoutMs: args.timeoutMs,
  headless: args.headless,
});
process.stdout.write(JSON.stringify(result) + '\n');
process.exit(result.ok ? 0 : 1);
