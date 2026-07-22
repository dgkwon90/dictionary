#!/usr/bin/env node
// @ts-check
/**
 * 크로스플랫폼 desktopd 사이드카 빌더 (#32).
 *
 * Tauri `beforeBuildCommand`/`beforeDevCommand`에서 호출된다. bash가 아니라 Node로 작성해
 * macOS(로컬)와 Windows(GitHub Actions 러너) 양쪽에서 동일하게 동작한다.
 *
 * 대상 타깃 = Tauri가 주입하는 `TAURI_ENV_TARGET_TRIPLE`(있으면), 없으면 `rustc --print host-tuple`.
 * `universal-apple-darwin`이면 arm64+amd64 둘 다 빌드한다(Tauri가 externalBin을 lipo).
 * 결과물: `src-tauri/binaries/desktopd-<triple>[.exe]` — Tauri `bundle.externalBin`이 기대하는 이름.
 * desktopd는 cgo-free(modernc sqlite·zalando keyring)라 `CGO_ENABLED=0`으로 순수 크로스컴파일된다.
 *
 * 실패 시 exit 1로 빌드를 중단한다(구 사이드카를 조용히 번들에 싣지 않도록, #31 codex 지적).
 */
import { execFileSync } from 'node:child_process';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const uiDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const desktopdDir = resolve(uiDir, '..', 'desktopd');
const binariesDir = join(uiDir, 'src-tauri', 'binaries');

/** Rust 타깃 triple -> Go [GOOS, GOARCH] */
const GO = {
  'aarch64-apple-darwin': ['darwin', 'arm64'],
  'x86_64-apple-darwin': ['darwin', 'amd64'],
  'x86_64-pc-windows-msvc': ['windows', 'amd64'],
  'aarch64-pc-windows-msvc': ['windows', 'arm64'],
  'x86_64-unknown-linux-gnu': ['linux', 'amd64'],
  'aarch64-unknown-linux-gnu': ['linux', 'arm64'],
};

function hostTriple() {
  return execFileSync('rustc', ['--print', 'host-tuple'], { encoding: 'utf8' }).trim();
}

function buildOne(triple) {
  const go = GO[triple];
  if (!go) throw new Error(`지원하지 않는 타깃 triple: ${triple}`);
  const [GOOS, GOARCH] = go;
  const out = join(binariesDir, `desktopd-${triple}${GOOS === 'windows' ? '.exe' : ''}`);
  console.log(`[build-sidecar] ${triple} (GOOS=${GOOS} GOARCH=${GOARCH}) -> ${out}`);
  execFileSync('go', ['build', '-trimpath', '-o', out, './cmd/desktopd'], {
    cwd: desktopdDir,
    stdio: 'inherit',
    env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: '0' },
  });
}

try {
  const triple = (process.env.TAURI_ENV_TARGET_TRIPLE || hostTriple()).trim();
  if (!triple) throw new Error('타깃 triple을 확인할 수 없습니다 (rustc/TAURI_ENV_TARGET_TRIPLE)');
  mkdirSync(binariesDir, { recursive: true });
  const targets =
    triple === 'universal-apple-darwin'
      ? ['aarch64-apple-darwin', 'x86_64-apple-darwin']
      : [triple];
  for (const t of targets) buildOne(t);
} catch (err) {
  console.error(`[build-sidecar] 실패: ${err.message} — 빌드 중단`);
  process.exit(1);
}
