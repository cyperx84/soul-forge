// Resolves and downloads the platform-matching soul-forge binary from the GitHub
// release that corresponds to this npm package's version. Dependency-free: uses
// Node's https for the download and the system `tar` to extract (releases ship as
// .tar.gz, and we only target darwin/linux, where `tar` is always present).

const fs = require('fs');
const os = require('os');
const path = require('path');
const https = require('https');
const crypto = require('crypto');
const { execFileSync } = require('child_process');

const REPO = 'cyperx84/soul-forge';
const pkg = require('../package.json');

// target maps Node's platform/arch to goreleaser's naming.
function target() {
  const platform = { darwin: 'darwin', linux: 'linux' }[process.platform];
  const arch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  if (!platform || !arch) {
    throw new Error(
      `soul-forge: unsupported platform ${process.platform}/${process.arch}. ` +
        `Prebuilt binaries exist for darwin/linux on amd64/arm64. ` +
        `Install from source instead: go install github.com/${REPO}@latest`
    );
  }
  return { platform, arch };
}

function binaryPath() {
  return path.join(__dirname, '..', 'vendor', 'soul-forge');
}

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

// verifyChecksum aborts the install unless `file` matches its entry in the
// release's checksums.txt manifest (goreleaser publishes one per release).
async function verifyChecksum(file, asset, version) {
  const sumsUrl = `https://github.com/${REPO}/releases/download/v${version}/checksums.txt`;
  const sumsTmp = path.join(os.tmpdir(), `soul-forge_${version}_checksums.txt`);
  await download(sumsUrl, sumsTmp);
  const sums = fs.readFileSync(sumsTmp, 'utf8');
  fs.rmSync(sumsTmp, { force: true });

  // checksums.txt lines are "<sha256>  <filename>".
  const expected = sums
    .split('\n')
    .map((line) => line.trim().split(/\s+/))
    .find(([, name]) => name === asset)?.[0];
  if (!expected) {
    throw new Error(`soul-forge: ${asset} not listed in checksums.txt for v${version}`);
  }

  const actual = sha256(file);
  if (actual !== expected) {
    fs.rmSync(file, { force: true });
    throw new Error(
      `soul-forge: checksum mismatch for ${asset}\n` +
        `  expected ${expected}\n` +
        `  got      ${actual}`
    );
  }
}

function download(url, dest, redirects = 0) {
  return new Promise((resolve, reject) => {
    if (redirects > 10) return reject(new Error('too many redirects'));
    https
      .get(url, { headers: { 'User-Agent': 'soul-forge-npm' } }, (res) => {
        // GitHub release assets redirect to a CDN — follow the Location header.
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume();
          return resolve(download(res.headers.location, dest, redirects + 1));
        }
        if (res.statusCode !== 200) {
          res.resume();
          return reject(new Error(`download failed: HTTP ${res.statusCode} for ${url}`));
        }
        const file = fs.createWriteStream(dest);
        res.pipe(file);
        file.on('finish', () => file.close(() => resolve()));
        file.on('error', reject);
      })
      .on('error', reject);
  });
}

// ensure returns the path to the binary, downloading and extracting it on first use.
async function ensure() {
  const bin = binaryPath();
  if (fs.existsSync(bin)) return bin;

  const { platform, arch } = target();
  const version = pkg.version;
  const asset = `soul-forge_${version}_${platform}_${arch}.tar.gz`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${asset}`;

  const vendor = path.dirname(bin);
  fs.mkdirSync(vendor, { recursive: true });

  const tmp = path.join(os.tmpdir(), asset);
  await download(url, tmp);
  await verifyChecksum(tmp, asset, version);
  execFileSync('tar', ['-xzf', tmp, '-C', vendor], { stdio: 'inherit' });
  fs.chmodSync(bin, 0o755);
  fs.rmSync(tmp, { force: true });

  if (!fs.existsSync(bin)) {
    throw new Error(`soul-forge: binary not found after extracting ${asset}`);
  }
  return bin;
}

module.exports = { ensure, binaryPath };
