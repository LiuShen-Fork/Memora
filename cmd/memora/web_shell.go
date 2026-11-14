package main

import "bytes"

// The generated frontend renders on the client: until the bundle has arrived
// and Vue has mounted, the page the browser was handed is empty, and on a slow
// connection that emptiness is the first thing a visitor sees. The skeleton
// below fills it in. It is written inside the application's root element, so
// Vue clears it the moment the app mounts — no script has to take it down.

// splashSkeleton is the same avatar ring the app shows while it is setting
// itself up, drawn with inline styles because no stylesheet has loaded yet.
// It carries no text: the language is not known before the app boots.
const splashSkeleton = `<div class="cf-splash" aria-hidden="true">` +
	`<div class="cf-splash-inner">` +
	`<div class="cf-splash-ring"></div>` +
	`<div class="cf-splash-avatar"><img src="/web-app-manifest-192x192.png" alt=""></div>` +
	`</div></div>` +
	`<style>` +
	`.cf-splash{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:#fff}` +
	`.cf-splash-inner{position:relative;width:6rem;height:6rem}` +
	`.cf-splash-ring,.cf-splash-avatar{position:absolute;inset:0;border-radius:999px}` +
	`.cf-splash-ring{border:2px solid transparent;border-left-color:#16a34a;border-right-color:#16a34a;` +
	`border-top-color:#2563eb;border-bottom-color:#2563eb;animation:cf-splash-spin .8s linear infinite}` +
	`.cf-splash-avatar{inset:.25rem;overflow:hidden;background:#f5f5f5}` +
	`.cf-splash-avatar img{width:100%;height:100%;object-fit:cover}` +
	`@keyframes cf-splash-spin{to{transform:rotate(360deg)}}` +
	`@media (prefers-color-scheme:dark){.cf-splash{background:#09090b}.cf-splash-avatar{background:#27272a}}` +
	`</style>`

// injectSplash puts the skeleton inside the application's root element. A
// shell without that element (an older bundle, or a page someone replaced) is
// served untouched rather than guessed at.
func injectSplash(page []byte) []byte {
	const marker = `id="__nuxt"`
	start := bytes.Index(page, []byte(marker))
	if start < 0 {
		return page
	}
	end := bytes.IndexByte(page[start:], '>')
	if end < 0 {
		return page
	}
	at := start + end + 1
	out := make([]byte, 0, len(page)+len(splashSkeleton))
	out = append(out, page[:at]...)
	out = append(out, splashSkeleton...)
	out = append(out, page[at:]...)
	return out
}
