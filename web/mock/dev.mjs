#!/usr/bin/env node
/**
 * Runs the dashboard against the mock API: one command, two processes.
 *
 *   pnpm dev:mock      # mock api on 8787, dashboard on http://127.0.0.1:3001
 *
 * Ctrl-C stops both. Pass MOCK_PORT to move the mock.
 */

import { spawn } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const web = join(dirname(fileURLToPath(import.meta.url)), '..')
const port = process.env.MOCK_PORT || '8787'

const children = []

function run(command, args, env) {
  const child = spawn(command, args, {
    cwd: web,
    stdio: 'inherit',
    shell: process.platform === 'win32',
    env: { ...process.env, ...env },
  })
  children.push(child)
  return child
}

run(process.execPath, ['mock/server.mjs'], { MOCK_PORT: port })
run('npx', ['nuxt', 'dev', '--host', '0.0.0.0', '--port', '3001'], {
  MEMORA_BACKEND_ORIGIN: `http://127.0.0.1:${port}`,
})

function stop() {
  for (const child of children) child.kill()
  process.exit(0)
}

process.on('SIGINT', stop)
process.on('SIGTERM', stop)
