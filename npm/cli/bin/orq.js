#!/usr/bin/env node
// @orq-ai/cli launcher shim.
//
// Resolves the matching per-platform package installed as an optional
// dependency, locates its bundled Go binary, and execs it with the same
// stdio / argv / exit code semantics as running the binary directly.

'use strict';

const { execFileSync } = require('child_process');
const path = require('path');

const platformPackages = {
  'darwin-arm64': '@orq-ai/cli-darwin-arm64',
  'darwin-x64':   '@orq-ai/cli-darwin-x64',
  'linux-x64':    '@orq-ai/cli-linux-x64',
  'linux-arm64':  '@orq-ai/cli-linux-arm64',
  'win32-x64':    '@orq-ai/cli-win32-x64',
};

const key = `${process.platform}-${process.arch}`;
const pkg = platformPackages[key];

if (!pkg) {
  console.error(`@orq-ai/cli: unsupported platform ${key}.`);
  console.error(`Supported platforms: ${Object.keys(platformPackages).join(', ')}.`);
  console.error('Open an issue at https://github.com/orq-ai/orq-cli/issues');
  process.exit(1);
}

let binaryPath;
try {
  const packageJsonPath = require.resolve(`${pkg}/package.json`);
  const exe = process.platform === 'win32' ? 'orq.exe' : 'orq';
  binaryPath = path.join(path.dirname(packageJsonPath), 'bin', exe);
} catch (err) {
  console.error(`@orq-ai/cli: the platform package ${pkg} was not installed.`);
  console.error('This can happen if --no-optional / --omit=optional was passed to npm.');
  console.error('Reinstall with:');
  console.error('  npm install -g @orq-ai/cli');
  process.exit(1);
}

try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (err) {
  if (err && typeof err.status === 'number') {
    process.exit(err.status);
  }
  if (err && err.code === 'ENOENT') {
    console.error(`@orq-ai/cli: binary not found at ${binaryPath}`);
    process.exit(1);
  }
  throw err;
}
