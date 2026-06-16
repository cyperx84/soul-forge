#!/usr/bin/env node
// Thin launcher: ensures the native soul-forge binary is present (downloading it on
// first run if needed), then execs it with the user's arguments and exit code.

const fs = require('fs');
const { spawnSync } = require('child_process');
const { ensure, binaryPath } = require('../lib/install');

(async () => {
  let bin = binaryPath();
  if (!fs.existsSync(bin)) {
    try {
      bin = await ensure();
    } catch (err) {
      console.error(err.message || String(err));
      process.exit(1);
    }
  }
  const res = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
  if (res.error) {
    console.error(res.error.message);
    process.exit(1);
  }
  process.exit(res.status === null ? 1 : res.status);
})();
