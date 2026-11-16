package main

import "bytes"

// The generated frontend renders on the client: until the bundle has arrived
// and Vue has mounted, the page the browser was handed is empty, and on a slow
// connection that emptiness is the first thing a visitor sees. The skeleton
// below fills it in. It is written inside the application's root element, so
// Vue clears it the moment the app mounts — no script has to take it down.

// splashSkeleton is the same loading mark the app draws while it is setting
// itself up — an arc sweeping around the site icon — painted with inline
// styles because no stylesheet has loaded yet. It carries no text: the
// language is not known before the app boots.
//
// Nothing is drawn behind the icon: the mark has transparent corners, so a
// plate or a ring around it reads as a border the icon does not have.
//
// The theme is keyed on the `dark` class the color-mode script writes onto
// <html> from the head, which has run by the time this element paints. The
// media query behind it is only the fallback for a browser that never ran that
// script, and it is written so an explicit choice wins over the machine: a
// visitor who picked dark in the app on a light machine used to get a white
// splash here, because the only dark rule was prefers-color-scheme.
const splashSkeleton = `<div class="cf-splash" aria-hidden="true">` +
	`<div class="cf-splash-inner">` +
	`<div class="cf-splash-glow"></div>` +
	`<svg class="cf-splash-arc" viewBox="0 0 96 96" fill="none">` +
	`<defs><linearGradient id="cf-splash-gradient" x1="8" y1="8" x2="88" y2="88" gradientUnits="userSpaceOnUse">` +
	`<stop class="cf-splash-from" offset="0%"/><stop class="cf-splash-to" offset="100%"/>` +
	`</linearGradient></defs>` +
	`<circle cx="48" cy="48" r="45" stroke="url(#cf-splash-gradient)" stroke-width="3" ` +
	`stroke-linecap="round" stroke-dasharray="66 217"/>` +
	`</svg>` +
	`<img class="cf-splash-mark" src="/web-app-manifest-192x192.png" alt="">` +
	`</div></div>` +
	`<style>` +
	`.cf-splash{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:#fff}` +
	`.cf-splash-inner{position:relative;width:6rem;height:6rem}` +
	`.cf-splash-glow,.cf-splash-arc,.cf-splash-mark{position:absolute;inset:0;width:100%;height:100%}` +
	`.cf-splash-glow{border-radius:999px;background:radial-gradient(circle at center,#ec4899,transparent 68%);` +
	`opacity:.16;filter:blur(6px);animation:cf-splash-breathe 2.8s ease-in-out infinite}` +
	`.cf-splash-arc{transform:rotate(-90deg);animation:cf-splash-orbit 1.3s cubic-bezier(.6,.15,.35,.85) infinite;` +
	`filter:drop-shadow(0 1px 6px rgb(0 0 0/.08))}` +
	`.cf-splash-mark{margin:auto;width:58%;height:58%;object-fit:contain;` +
	`animation:cf-splash-breathe 2.8s ease-in-out infinite;filter:drop-shadow(0 2px 8px rgb(0 0 0/.12))}` +
	`.cf-splash-from{stop-color:#ec4899}` +
	`.cf-splash-to{stop-color:#a78bfa}` +
	`@keyframes cf-splash-orbit{from{transform:rotate(-90deg)}to{transform:rotate(270deg)}}` +
	`@keyframes cf-splash-breathe{0%,100%{transform:scale(.97)}50%{transform:scale(1.03)}}` +
	`@media (prefers-color-scheme:dark){html:not(.light) .cf-splash{background:#171717}` +
	`html:not(.light) .cf-splash-from{stop-color:#f472b6}` +
	`html:not(.light) .cf-splash-glow{background:radial-gradient(circle at center,#f472b6,transparent 68%)}}` +
	`html.dark .cf-splash{background:#171717}` +
	`html.dark .cf-splash-from{stop-color:#f472b6}` +
	`html.dark .cf-splash-glow{background:radial-gradient(circle at center,#f472b6,transparent 68%)}` +
	`@media (prefers-reduced-motion:reduce){.cf-splash-glow,.cf-splash-arc,.cf-splash-mark{animation:none}}` +
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
