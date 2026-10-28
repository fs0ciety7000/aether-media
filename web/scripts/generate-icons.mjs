// Renders the PWA icons (web/static/icons) from the Aether monogram (same paths as Logo.svelte
// and favicon.svg). Needs Playwright (Chromium): node scripts/generate-icons.mjs
import { createRequire } from 'node:module';
import { mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const out = join(dirname(fileURLToPath(import.meta.url)), '..', 'static', 'icons');
mkdirSync(out, { recursive: true });

const BG = '#04121D';
const FRONT = '#5EC4C0';
const BACK = '#38797B';
const mark = `<path d="M13 4h6l10 24h-6Z" fill="${BACK}"/><path d="M13 4h6L9 28H3Z" fill="${FRONT}"/><path d="M12.54 19.5h6.92l1.25 3H11.29Z" fill="${BACK}"/>`;

/** scale = share of the canvas taken by the 32-unit mark; radius = corner radius (share of size). */
function svg(size, { scale, radius, glow }) {
	const s = (size * scale) / 32;
	const off = (size - 32 * s) / 2;
	const r = size * radius;
	return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">
	<defs><radialGradient id="g" cx="20%" cy="12%" r="80%"><stop offset="0" stop-color="${FRONT}" stop-opacity="${glow}"/><stop offset="1" stop-color="${FRONT}" stop-opacity="0"/></radialGradient></defs>
	<rect width="${size}" height="${size}" rx="${r}" fill="${BG}"/>
	<rect width="${size}" height="${size}" rx="${r}" fill="url(#g)"/>
	<g transform="translate(${off} ${off}) scale(${s})">${mark}</g>
</svg>`;
}

const ICONS = [
	{ file: 'icon-192.png', size: 192, scale: 0.62, radius: 0.22, glow: 0.14 },
	{ file: 'icon-512.png', size: 512, scale: 0.62, radius: 0.22, glow: 0.14 },
	// Maskable: full bleed, the mark inside the 80 % safe zone.
	{ file: 'maskable-512.png', size: 512, scale: 0.46, radius: 0, glow: 0.12 },
	{ file: 'maskable-192.png', size: 192, scale: 0.46, radius: 0, glow: 0.12 },
	// iOS rounds the corners itself and refuses transparency.
	{ file: 'apple-touch-icon.png', size: 180, scale: 0.6, radius: 0, glow: 0.14 },
	{ file: 'favicon-32.png', size: 32, scale: 0.72, radius: 0.22, glow: 0 }
];

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const icon of ICONS) {
	await page.setViewportSize({ width: icon.size, height: icon.size });
	await page.setContent(`<html><body style="margin:0;background:transparent">${svg(icon.size, icon)}</body></html>`);
	await page.locator('svg').screenshot({ path: join(out, icon.file), omitBackground: true });
	console.log('icons/' + icon.file);
}
await browser.close();
