#!/usr/bin/env node
// Verifies WCAG contrast ratios for every theme declared in src/lib/styles/tokens.css.
// Exits with code 1 when a required pair falls below its threshold.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const tokensPath = fileURLToPath(new URL('../src/lib/styles/tokens.css', import.meta.url));
const css = readFileSync(tokensPath, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');

/** Required pairs: [foreground, background, minimum ratio, note]. */
const PAIRS = [
	['--text', '--bg', 4.5, 'texte courant'],
	['--text', '--surface', 4.5, 'texte sur carte'],
	['--text', '--surface-raised', 4.5, 'texte sur menu'],
	['--text-muted', '--bg', 4.5, 'métadonnées'],
	['--text-muted', '--surface', 4.5, 'métadonnées sur carte'],
	['--accent-contrast', '--accent', 4.5, 'bouton primaire'],
	['--accent', '--bg', 3, 'focus, progression (non-texte)'],
	['--text-faint', '--bg', 3, 'grand texte / placeholder'],
	['--accent-ink', '--accent-soft@--surface', 4.5, 'chip sélectionnée'],
	['--accent-ink', '--bg', 4.5, 'lien accentué']
];

// Innermost rule blocks only (works inside @media too).
const themes = new Map();
for (const [, selector, body] of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
	const names = [];
	for (const part of selector.split(',').map((s) => s.trim())) {
		const m = part.match(/data-theme=['"]([\w-]+)['"]/);
		if (m && m[1] !== 'system') names.push(m[1]);
		else if (part === ':root' && /--bg\s*:/.test(body)) names.push('salle-obscure');
	}
	if (!names.length) continue;
	const vars = {};
	for (const [, name, value] of body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) vars[name] = value.trim();
	for (const n of names) themes.set(n, { ...(themes.get(n) ?? {}), ...vars });
}

/** Resolves a token, compositing translucent colours: "--a@--b" = --a over --b. */
function resolve(vars, ref) {
	const [top, under] = ref.split('@');
	const value = vars[top];
	if (value === undefined || (under && vars[under] === undefined)) return null;
	const color = parseColor(value);
	const alpha = parseAlpha(value);
	if (!under || alpha === 1) return color;
	const base = resolve(vars, under);
	return color.map((c, i) => Math.round(c * alpha + base[i] * (1 - alpha)));
}

function parseAlpha(value) {
	const m = value.match(/^rgba\([^)]*,\s*([\d.]+)\s*\)$/i);
	return m ? Number(m[1]) : 1;
}

function parseColor(value) {
	const hex = value.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i);
	if (hex) {
		let h = hex[1];
		if (h.length === 3) h = [...h].map((c) => c + c).join('');
		return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
	}
	const rgb = value.match(/^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/i);
	if (rgb) return rgb.slice(1, 4).map(Number);
	throw new Error(`couleur non prise en charge : ${value}`);
}

function luminance([r, g, b]) {
	const lin = (c) => {
		const s = c / 255;
		return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
	};
	return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function ratio(a, b) {
	const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (l1 + 0.05) / (l2 + 0.05);
}

const expected = ['salle-obscure', 'velours', 'ardoise', 'noyer', 'lin'];
let failures = 0;

for (const name of expected) {
	const vars = themes.get(name);
	if (!vars) {
		console.error(`✗ thème introuvable : ${name}`);
		failures++;
		continue;
	}
	console.log(`\n${name}`);
	for (const [fg, bg, min, note] of PAIRS) {
		const a = resolve(vars, fg);
		const b = resolve(vars, bg);
		if (!a || !b) {
			console.error(`  ✗ ${fg} / ${bg} : token manquant`);
			failures++;
			continue;
		}
		const r = ratio(a, b);
		const ok = r >= min;
		if (!ok) failures++;
		const line = `  ${ok ? '✓' : '✗'} ${fg.padEnd(17)} / ${bg.padEnd(22)} ${r.toFixed(2).padStart(5)}:1  (≥ ${min}, ${note})`;
		(ok ? console.log : console.error)(line);
	}
}

if (failures) {
	console.error(`\n${failures} contraste(s) insuffisant(s).`);
	process.exit(1);
}
console.log('\nTous les contrastes sont conformes.');
