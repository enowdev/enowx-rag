// postinstall: download the platform-native enowx-rag binary from the matching
// GitHub Release and place it at bin/enowx-rag(.exe). No runtime npm deps —
// uses Node's built-in https and the system tar/unzip for extraction.
'use strict'

const fs = require('fs')
const os = require('os')
const path = require('path')
const https = require('https')
const { execFileSync } = require('child_process')
const { resolveTarget, binaryName, binaryPath, archiveFor } = require('./lib/platform')

const REPO = 'enowdev/enowx-rag'
const version = 'v' + require('./package.json').version

function log(msg) {
  process.stdout.write(`enowx-rag: ${msg}\n`)
}

// Follow redirects (GitHub release assets 302 to a CDN) and stream to a file.
function download(url, dest, redirects = 0) {
  return new Promise((resolve, reject) => {
    if (redirects > 10) return reject(new Error('too many redirects'))
    https
      .get(url, { headers: { 'User-Agent': 'enowx-rag-npm' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume()
          return resolve(download(res.headers.location, dest, redirects + 1))
        }
        if (res.statusCode !== 200) {
          res.resume()
          return reject(new Error(`GET ${url} -> HTTP ${res.statusCode}`))
        }
        const file = fs.createWriteStream(dest)
        res.pipe(file)
        file.on('finish', () => file.close(resolve))
        file.on('error', reject)
      })
      .on('error', reject)
  })
}

function extract(archive, destDir, isZip) {
  if (isZip) {
    // Windows 10+ ships tar.exe which handles zip; fall back to Expand-Archive.
    try {
      execFileSync('tar', ['-xf', archive, '-C', destDir], { stdio: 'ignore' })
    } catch {
      execFileSync(
        'powershell',
        ['-NoProfile', '-Command', `Expand-Archive -Force -Path '${archive}' -DestinationPath '${destDir}'`],
        { stdio: 'ignore' }
      )
    }
  } else {
    execFileSync('tar', ['-xzf', archive, '-C', destDir], { stdio: 'ignore' })
  }
}

async function main() {
  // Allow skipping the download (e.g. air-gapped CI that provides its own binary).
  if (process.env.ENOWX_SKIP_DOWNLOAD === '1') {
    log('ENOWX_SKIP_DOWNLOAD=1 set; skipping binary download')
    return
  }

  const { os: goos, arch } = resolveTarget()
  const archive = archiveFor(version, goos, arch)
  const url = `https://github.com/${REPO}/releases/download/${version}/${archive}`

  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'enowx-rag-'))
  const archivePath = path.join(tmp, archive)
  const binDir = path.join(__dirname, 'bin')
  fs.mkdirSync(binDir, { recursive: true })

  try {
    log(`downloading ${archive} (${version})…`)
    await download(url, archivePath)
    extract(archivePath, tmp, archive.endsWith('.zip'))

    const extracted = path.join(tmp, binaryName())
    if (!fs.existsSync(extracted)) {
      throw new Error(`archive did not contain ${binaryName()}`)
    }
    const target = binaryPath()
    fs.copyFileSync(extracted, target)
    if (process.platform !== 'win32') fs.chmodSync(target, 0o755)
    log(`installed ${target}`)
  } catch (err) {
    log(`failed to download binary: ${err.message}`)
    log(`you can install manually from https://github.com/${REPO}/releases/tag/${version}`)
    process.exitCode = 1
  } finally {
    fs.rmSync(tmp, { recursive: true, force: true })
  }
}

main()
