// Shared platform resolution for the enowx-rag npm wrapper.
// Maps Node's process.platform/arch to the GoReleaser archive naming and the
// on-disk binary path, so install.js and the bin launcher agree.
'use strict'

const path = require('path')

// Node platform/arch -> GoReleaser goos/goarch (must match .goreleaser.yaml).
const OS_MAP = { darwin: 'darwin', linux: 'linux', win32: 'windows' }
const ARCH_MAP = { x64: 'amd64', arm64: 'arm64' }

function resolveTarget() {
  const os = OS_MAP[process.platform]
  const arch = ARCH_MAP[process.arch]
  if (!os || !arch) {
    throw new Error(
      `unsupported platform: ${process.platform}/${process.arch}. ` +
        `Install a prebuilt binary from https://github.com/enowdev/enowx-rag/releases`
    )
  }
  // GoReleaser skips windows/arm64.
  if (os === 'windows' && arch === 'arm64') {
    throw new Error('windows/arm64 is not built; use amd64 or install from source')
  }
  return { os, arch }
}

function binaryName() {
  return process.platform === 'win32' ? 'enowx-rag.exe' : 'enowx-rag'
}

// Where the downloaded binary is cached inside the installed package.
function binaryPath() {
  return path.join(__dirname, '..', 'bin', binaryName())
}

// Archive filename + format for a given version (GoReleaser name_template).
function archiveFor(version, os, arch) {
  const v = version.replace(/^v/, '')
  const ext = os === 'windows' ? 'zip' : 'tar.gz'
  return `enowx-rag_${v}_${os}_${arch}.${ext}`
}

module.exports = { resolveTarget, binaryName, binaryPath, archiveFor }
