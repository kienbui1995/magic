#!/usr/bin/env node
import { spawnSync } from 'child_process';
import { createWriteStream, chmodSync, existsSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import https from 'https';

const __dirname = dirname(fileURLToPath(import.meta.url));
const BINARY = join(__dirname, 'magic-bin');
const VERSION = '0.3.1';
const REPO = 'kienbui1995/magic';

function getPlatform() {
  const p = process.platform, a = process.arch;
  if (p === 'darwin' && a === 'x64')   return 'darwin-amd64';
  if (p === 'darwin' && a === 'arm64') return 'darwin-arm64';
  if (p === 'linux'  && a === 'x64')   return 'linux-amd64';
  if (p === 'linux'  && a === 'arm64') return 'linux-arm64';
  throw new Error(`Unsupported platform: ${p}-${a}`);
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = createWriteStream(dest);
    const get = (u) => https.get(u, (res) => {
      if (res.statusCode === 301 || res.statusCode === 302) return get(res.headers.location);
      if (res.statusCode !== 200) return reject(new Error(`HTTP ${res.statusCode}`));
      res.pipe(file);
      file.on('finish', () => file.close(resolve));
    }).on('error', reject);
    get(url);
  });
}

if (!existsSync(BINARY)) {
  const platform = getPlatform();
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/magic-${platform}`;
  process.stderr.write(`Downloading MagiC binary for ${platform}...\n`);
  await download(url, BINARY);
  chmodSync(BINARY, 0o755);
  process.stderr.write('Done.\n');
}

const result = spawnSync(BINARY, process.argv.slice(2), { stdio: 'inherit' });
process.exit(result.status ?? 1);
