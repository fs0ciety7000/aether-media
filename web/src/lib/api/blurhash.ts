// Small BlurHash decoder (https://blurha.sh, MIT algorithm) → cached PNG data URL.
// Decoded at 32 px: the browser upscales it smoothly, which is all a placeholder needs.

const DIGITS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz#$%*+,-.:;=?@[]^_{|}~';

function decode83(str: string): number {
	let v = 0;
	for (const c of str) {
		const d = DIGITS.indexOf(c);
		if (d < 0) throw new Error('blurhash: invalid character');
		v = v * 83 + d;
	}
	return v;
}

function sRGBToLinear(v: number): number {
	const x = v / 255;
	return x <= 0.04045 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4);
}

function linearToSRGB(v: number): number {
	const x = Math.max(0, Math.min(1, v));
	return x <= 0.0031308 ? Math.round(x * 12.92 * 255 + 0.5) : Math.round((1.055 * Math.pow(x, 1 / 2.4) - 0.055) * 255 + 0.5);
}

const signPow = (v: number, e: number) => Math.sign(v) * Math.pow(Math.abs(v), e);

/** Decodes to RGBA pixels. Throws on a malformed hash. */
export function decodeBlurhash(hash: string, width: number, height: number, punch = 1): Uint8ClampedArray {
	if (!hash || hash.length < 6) throw new Error('blurhash: too short');
	const size = decode83(hash[0]);
	const nx = (size % 9) + 1;
	const ny = Math.floor(size / 9) + 1;
	if (hash.length !== 4 + 2 * nx * ny) throw new Error('blurhash: bad length');

	const maxAc = (decode83(hash[1]) + 1) / 166;
	const colors: [number, number, number][] = new Array(nx * ny);
	const dc = decode83(hash.slice(2, 6));
	colors[0] = [sRGBToLinear(dc >> 16), sRGBToLinear((dc >> 8) & 255), sRGBToLinear(dc & 255)];
	for (let i = 1; i < nx * ny; i++) {
		const v = decode83(hash.slice(4 + i * 2, 6 + i * 2));
		const q = (n: number) => signPow((n - 9) / 9, 2) * maxAc * punch;
		colors[i] = [q(Math.floor(v / 361)), q(Math.floor(v / 19) % 19), q(v % 19)];
	}

	const px = new Uint8ClampedArray(width * height * 4);
	const cosX = new Float64Array(width * nx);
	const cosY = new Float64Array(height * ny);
	for (let x = 0; x < width; x++) for (let i = 0; i < nx; i++) cosX[x * nx + i] = Math.cos((Math.PI * x * i) / width);
	for (let y = 0; y < height; y++) for (let j = 0; j < ny; j++) cosY[y * ny + j] = Math.cos((Math.PI * y * j) / height);

	for (let y = 0; y < height; y++) {
		for (let x = 0; x < width; x++) {
			let r = 0,
				g = 0,
				b = 0;
			for (let j = 0; j < ny; j++) {
				const cy = cosY[y * ny + j];
				for (let i = 0; i < nx; i++) {
					const basis = cosX[x * nx + i] * cy;
					const c = colors[i + j * nx];
					r += c[0] * basis;
					g += c[1] * basis;
					b += c[2] * basis;
				}
			}
			const o = 4 * (x + y * width);
			px[o] = linearToSRGB(r);
			px[o + 1] = linearToSRGB(g);
			px[o + 2] = linearToSRGB(b);
			px[o + 3] = 255;
		}
	}
	return px;
}

const cache = new Map<string, string | null>();
const MAX_CACHE = 400;

/** Data URL of the decoded hash (cached), or undefined if it cannot be decoded here. */
export function blurhashToDataURL(hash: string | undefined, ratio = 1): string | undefined {
	if (!hash || typeof document === 'undefined') return undefined;
	// Bucket the aspect so a poster and a backdrop of the same hash don't collide.
	const w = ratio >= 1 ? 32 : Math.max(8, Math.round(32 * ratio));
	const h = ratio >= 1 ? Math.max(8, Math.round(32 / ratio)) : 32;
	const key = `${hash}|${w}x${h}`;
	const hit = cache.get(key);
	if (hit !== undefined) return hit ?? undefined;
	let url: string | null = null;
	try {
		const canvas = document.createElement('canvas');
		canvas.width = w;
		canvas.height = h;
		const ctx = canvas.getContext('2d');
		if (ctx) {
			const data = ctx.createImageData(w, h);
			data.data.set(decodeBlurhash(hash, w, h));
			ctx.putImageData(data, 0, 0);
			url = canvas.toDataURL('image/png');
		}
	} catch {
		url = null;
	}
	if (cache.size >= MAX_CACHE) cache.delete(cache.keys().next().value!);
	cache.set(key, url);
	return url ?? undefined;
}
