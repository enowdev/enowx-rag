#!/usr/bin/env node
// Launcher shim: exec the native enowx-rag binary downloaded by postinstall,
// forwarding all arguments, stdio, and the exit code.
'use strict'

const fs = require('fs')
const { spawnSync } = require('child_process')
const { binaryPath } = require('../lib/platform')

const bin = binaryPath()
if (!fs.existsSync(bin)) {
  process.stderr.write(
    'enowx-rag: native binary not found. The postinstall download may have failed.\n' +
      'Reinstall (npm install -g enowx-rag) or download from ' +
      'https://github.com/enowdev/enowx-rag/releases\n'
  )
  process.exit(1)
}

const res = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' })
if (res.error) {
  process.stderr.write(`enowx-rag: ${res.error.message}\n`)
  process.exit(1)
}
process.exit(res.status === null ? 1 : res.status)
