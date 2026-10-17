#!/usr/bin/env node
/**
 * Screenshot a page through the Chrome DevTools Protocol, with no dependencies.
 *
 *   node mock/shoot.mjs <url> <out.png> [options]
 *
 *   --width N      viewport width           (default 1440)
 *   --height N     viewport height          (default 1000)
 *   --scale N      device scale factor      (default 1)
 *   --full         capture the whole page, not just the viewport
 *   --wait N       extra milliseconds after load (default 2500)
 *   --until CSS    wait for this selector before the extra wait
 *   --theme dark   dark | light | system    (default system)
 *   --port N       remote debugging port    (default 9222)
 *
 * Console errors, page exceptions and failed requests are printed to stderr,
 * which is the point of driving a real browser: a screenshot can look plausible
 * while the page underneath it is throwing.
 */

import { spawn } from 'node:child_process'
import { existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { tmpdir } from 'node:os'

const CHROME_CANDIDATES = [
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
]

function parseArgs(argv) {
  const options = {
    url: null,
    out: null,
    width: 1440,
    height: 1000,
    scale: 1,
    full: false,
    wait: 2500,
    until: null,
    theme: 'system',
    port: 9222,
  }
  const rest = []
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    const next = () => argv[++i]
    if (arg === '--width') options.width = Number(next())
    else if (arg === '--height') options.height = Number(next())
    else if (arg === '--scale') options.scale = Number(next())
    else if (arg === '--full') options.full = true
    else if (arg === '--wait') options.wait = Number(next())
    else if (arg === '--until') options.until = next()
    else if (arg === '--theme') options.theme = next()
    else if (arg === '--port') options.port = Number(next())
    else if (arg === '--eval') options.eval = next()
    else rest.push(arg)
  }
  options.url = rest[0]
  options.out = rest[1]
  return options
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function waitForDevTools(port, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/version`)
      if (response.ok) return await response.json()
    } catch {
      // not up yet
    }
    await sleep(250)
  }
  throw new Error(`Chrome did not open a debugging port on ${port}`)
}

async function ensureChrome(port) {
  try {
    await fetch(`http://127.0.0.1:${port}/json/version`)
    return null
  } catch {
    // start our own
  }
  const binary = CHROME_CANDIDATES.find((path) => existsSync(path))
  if (!binary) throw new Error('No Chrome binary found')
  const profile = join(tmpdir(), `memora-shoot-${port}`)
  mkdirSync(profile, { recursive: true })
  const child = spawn(
    binary,
    [
      '--headless=new',
      `--remote-debugging-port=${port}`,
      `--user-data-dir=${profile}`,
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-gpu',
      '--hide-scrollbars',
      '--disable-extensions',
      '--force-color-profile=srgb',
      'about:blank',
    ],
    { stdio: 'ignore', detached: false },
  )
  child.on('error', () => {})
  await waitForDevTools(port)
  return child
}

class Cdp {
  constructor(socket) {
    this.socket = socket
    this.nextId = 1
    this.pending = new Map()
    this.handlers = new Map()
    socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      if (message.id && this.pending.has(message.id)) {
        const { resolve, reject } = this.pending.get(message.id)
        this.pending.delete(message.id)
        if (message.error) reject(new Error(JSON.stringify(message.error)))
        else resolve(message.result)
        return
      }
      const handler = this.handlers.get(message.method)
      if (handler) handler(message.params)
    })
  }

  send(method, params = {}) {
    const id = this.nextId++
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.socket.send(JSON.stringify({ id, method, params }))
    })
  }

  on(method, handler) {
    this.handlers.set(method, handler)
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  if (!options.url || !options.out) {
    console.error(
      'usage: node mock/shoot.mjs <url> <out.png> [--width N] [--height N] [--scale N] [--full] [--wait N] [--until CSS] [--theme dark|light]',
    )
    process.exit(2)
  }

  const ownChrome = await ensureChrome(options.port)
  const target = await (
    await fetch(`http://127.0.0.1:${options.port}/json/new?about:blank`, { method: 'PUT' })
  ).json()

  const socket = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((resolve, reject) => {
    socket.addEventListener('open', resolve)
    socket.addEventListener('error', reject)
  })
  const cdp = new Cdp(socket)

  const problems = []
  cdp.on('Runtime.consoleAPICalled', ({ type, args }) => {
    if (type !== 'error' && type !== 'warning') return
    const text = args
      .map((arg) => arg.value ?? arg.description ?? arg.unserializableValue ?? '')
      .join(' ')
      .slice(0, 300)
    problems.push(`[console.${type}] ${text}`)
  })
  cdp.on('Runtime.exceptionThrown', ({ exceptionDetails }) => {
    problems.push(
      `[exception] ${exceptionDetails.exception?.description || exceptionDetails.text}`.slice(0, 400),
    )
  })
  cdp.on('Network.loadingFailed', ({ errorText, type }) => {
    if (errorText === 'net::ERR_ABORTED') return
    problems.push(`[request failed] ${type} ${errorText}`)
  })
  cdp.on('Network.responseReceived', ({ response }) => {
    if (response.status >= 400) {
      problems.push(`[http ${response.status}] ${response.url.slice(0, 160)}`)
    }
  })

  await cdp.send('Page.enable')
  await cdp.send('Runtime.enable')
  await cdp.send('Network.enable')
  await cdp.send('Emulation.setDeviceMetricsOverride', {
    width: options.width,
    height: options.height,
    deviceScaleFactor: options.scale,
    mobile: false,
  })
  if (options.theme !== 'system') {
    await cdp.send('Emulation.setEmulatedMedia', {
      features: [{ name: 'prefers-color-scheme', value: options.theme }],
    })
  }

  const loaded = new Promise((resolve) => cdp.on('Page.loadEventFired', resolve))
  await cdp.send('Page.navigate', { url: options.url })
  await Promise.race([loaded, sleep(20000)])

  if (options.until) {
    await cdp
      .send('Runtime.evaluate', {
        expression: `new Promise((r) => {
          const found = document.querySelector(${JSON.stringify(options.until)})
          if (found) return r(true)
          const observer = new MutationObserver(() => {
            if (document.querySelector(${JSON.stringify(options.until)})) {
              observer.disconnect(); r(true)
            }
          })
          observer.observe(document.documentElement, { childList: true, subtree: true })
          setTimeout(() => { observer.disconnect(); r(false) }, 15000)
        })`,
        awaitPromise: true,
        returnByValue: true,
      })
      .catch(() => {})
  }

  await sleep(options.wait)

  if (options.full) {
    const measured = await cdp.send('Runtime.evaluate', {
      expression:
        'Math.max(document.body.scrollHeight, document.documentElement.scrollHeight, window.innerHeight)',
      returnByValue: true,
    })
    await cdp.send('Emulation.setDeviceMetricsOverride', {
      width: options.width,
      height: Math.min(20000, Math.ceil(measured.result.value)),
      deviceScaleFactor: options.scale,
      mobile: false,
    })
    await sleep(400)
  }

  // The eval runs before the picture is taken so that it can also be used to
  // put the page into a state worth photographing — opening a menu, expanding a
  // row — and not only to read numbers out of it.
  if (options.eval) {
    const result = await cdp.send('Runtime.evaluate', {
      expression: options.eval,
      returnByValue: true,
      awaitPromise: true,
    })
    console.log(
      typeof result.result.value === 'string'
        ? result.result.value
        : JSON.stringify(result.result.value, null, 2),
    )
    await sleep(400)
  }

  const shot = await cdp.send('Page.captureScreenshot', { format: 'png' })
  mkdirSync(dirname(options.out), { recursive: true })
  writeFileSync(options.out, Buffer.from(shot.data, 'base64'))

  socket.close()
  await fetch(`http://127.0.0.1:${options.port}/json/close/${target.id}`).catch(() => {})
  if (ownChrome) ownChrome.kill()

  if (problems.length) {
    const unique = [...new Set(problems)]
    console.error(`--- ${unique.length} problem(s) ---`)
    for (const problem of unique.slice(0, 25)) console.error(problem)
  }
  console.log(`${options.out} (${options.width}x${options.full ? 'full' : options.height})`)
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
