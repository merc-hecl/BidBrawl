import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { assetNames, checksums, platforms } from './assets.mjs';

test('release requires all ten assets and rejects missing, extra, or empty files', t => {
  const dir = mkdtempSync(join(tmpdir(), 'bidbrawl-release-assets-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const names = platforms.flatMap(platform => Object.values(assetNames('0.1.0', platform)));
  assert.equal(new Set(names).size, 10);
  for (const name of names.slice(1)) writeFileSync(join(dir, name), 'abc');
  assert.throws(() => checksums('0.1.0', dir), /missing or unexpected/);
  writeFileSync(join(dir, names[0]), '');
  assert.throws(() => checksums('0.1.0', dir), /Empty release asset/);
  writeFileSync(join(dir, names[0]), 'abc');
  checksums('0.1.0', dir);
  const expected = [...names].sort().map(name => `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  ${name}`).join('\n') + '\n';
  assert.equal(readFileSync(join(dir, 'SHA256SUMS'), 'utf8'), expected);
  checksums('0.1.0', dir);
  assert.equal(readFileSync(join(dir, 'SHA256SUMS'), 'utf8'), expected);
  writeFileSync(join(dir, 'old-build.exe'), 'abc');
  assert.throws(() => checksums('0.1.0', dir), /missing or unexpected/);
});
