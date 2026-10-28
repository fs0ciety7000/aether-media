// Image URL helpers. Server variants exist only for normalised widths (docs/API.md).
import type { Image, ImageType, Item } from './types';

export const IMAGE_WIDTHS = [92, 154, 185, 342, 500, 780, 1280, 1920, 3840] as const;

function dpr(): number {
	return typeof window === 'undefined' ? 1 : Math.min(window.devicePixelRatio || 1, 3);
}

/** Smallest normalised width ≥ `px` (the largest one past the end). */
export function snapWidth(px: number): number {
	for (const w of IMAGE_WIDTHS) if (w >= px) return w;
	return IMAGE_WIDTHS[IMAGE_WIDTHS.length - 1];
}

function withWidth(url: string, w: number): string {
	return `${url}${url.includes('?') ? '&' : '?'}w=${w}`;
}

/**
 * URL of a variant at least `width` CSS pixels wide on this screen
 * (devicePixelRatio applied), never wider than the source image.
 */
export function imageUrl(img: Image | undefined | null, width: number): string | undefined {
	if (!img?.url) return undefined;
	let w = snapWidth(width * dpr());
	// The server never upscales: past the source width, every request yields the same file.
	if (img.width && w > img.width) w = snapWidth(img.width);
	return withWidth(img.url, w);
}

/** `srcset` by width, from `min` up to `max` (and never past the source width). */
export function imageSrcset(img: Image | undefined | null, min = 92, max = 1920): string | undefined {
	if (!img?.url) return undefined;
	const limit = img.width ? snapWidth(img.width) : max;
	const widths: number[] = IMAGE_WIDTHS.filter((w) => w >= min && w <= max && w <= limit);
	if (!widths.length) widths.push(snapWidth(min));
	return widths.map((w) => `${withWidth(img.url, w)} ${w}w`).join(', ');
}

/** First image of a type, with optional fallbacks: firstImage(it, 'thumb', 'backdrop'). */
export function firstImage(item: Pick<Item, 'images'> | undefined | null, ...types: ImageType[]): Image | undefined {
	if (!item?.images) return undefined;
	for (const t of types) {
		const img = item.images[t]?.[0];
		if (img?.url) return img;
	}
	return undefined;
}
