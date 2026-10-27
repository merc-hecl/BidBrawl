import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const platforms = ['windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64'];

export function assetNames(version, platform) {
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) throw new Error('Expected a stable mygo.json version');
  if (!platforms.includes(platform)) throw new Error(`Unsupported release platform: ${platform}`);
  const [os, arch] = platform.split('/');
  const installer = os === 'windows' ? `BidBrawl-Setup-${version}-${arch}.exe`
    : os === 'linux' ? `bidbrawl_${version}_${arch}.deb` : `BidBrawl-${version}-${arch}.pkg`;
  return { installer, archive: `bidbrawl-${version}-${os}-${arch}-cli.${os === 'windows' ? 'zip' : 'tar.gz'}` };
}

export function pack(platform) {
  const { version, out } = JSON.parse(readFileSync('mygo.json', 'utf8'));
  const { installer, archive } = assetNames(version, platform);
  const [os, arch] = platform.split('/');
  const hostOS = process.platform === 'win32' ? 'windows' : process.platform;
  if (hostOS !== os) throw new Error('Package release archives on the target OS to preserve executable permissions');
  const source = join(out, `${os}-${arch}`);
  const resources = os === 'darwin' ? join(source, 'BidBrawl.app', 'Contents', 'Resources') : source;
  const dest = resolve('build/release');
  mkdirSync(dest, { recursive: true });
  copyFileSync(join(source, installer), join(dest, installer));
  const stage = mkdtempSync(resolve('build/cli-'));
  try {
    const binary = os === 'windows' ? 'bidbrawl.exe' : 'bidbrawl';
    copyFileSync(join(resources, 'bin', binary), join(stage, binary));
    chmodSync(join(stage, binary), 0o755);
    for (const name of ['LICENSE', 'README.md', 'README.zh-CN.md']) copyFileSync(name, join(stage, name));
    copyFileSync(join(resources, 'THIRD_PARTY_LICENSES.txt'), join(stage, 'THIRD_PARTY_LICENSES.txt'));
    // Native tar preserves executable permissions; Windows ships bsdtar with ZIP support.
    execFileSync('tar', [os === 'windows' ? '-acf' : '-czf', join(dest, archive), '-C', stage, '.'], { stdio: 'inherit' });
  } finally {
    rmSync(stage, { recursive: true, force: true });
  }
}

export function checksums(version, directory) {
  const expected = platforms.flatMap(platform => Object.values(assetNames(version, platform))).sort();
  const actual = readdirSync(directory).filter(name => name !== 'SHA256SUMS').sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) throw new Error('Release assets are missing or unexpected');
  const lines = expected.map(name => {
    const path = join(directory, name);
    if (!statSync(path).isFile() || statSync(path).size === 0) throw new Error(`Empty release asset: ${name}`);
    return `${createHash('sha256').update(readFileSync(path)).digest('hex')}  ${name}`;
  });
  writeFileSync(join(directory, 'SHA256SUMS'), lines.join('\n') + '\n');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [command, platform, ...extra] = process.argv.slice(2);
  if (command === 'pack' && platform && !extra.length) pack(platform);
  else if (command === 'checksums' && !platform) checksums(JSON.parse(readFileSync('mygo.json', 'utf8')).version, 'build/release');
  else throw new Error('Usage: node .github/release/assets.mjs pack <os/arch> | checksums');
}
