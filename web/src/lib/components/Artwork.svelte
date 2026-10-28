<script lang="ts">
	// Image holder. Paints the dominant colour, then the decoded blurhash, then fades the real
	// image in (lazy, srcset by width). Without an image: generated art, or the title typeset.
	import type { Snippet } from 'svelte';
	import { blurhashToDataURL } from '$lib/api/blurhash';
	import { imageSrcset, imageUrl } from '$lib/api/images';
	import type { Image } from '$lib/api/types';
	import type { IconName } from './Icon.svelte';
	import TitleArt from './TitleArt.svelte';

	interface Props {
		/** API image (url + blurhash + dominant). */
		image?: Image;
		/** Plain image URL (legacy / demo). */
		src?: string;
		alt?: string;
		color?: string;
		/** Generated artwork, used when there is no image. */
		art?: Snippet;
		/** CSS aspect ratio; omit with `fill` to cover the parent instead. */
		ratio?: string;
		/** Absolutely fill the positioned parent (hero, headers). */
		fill?: boolean;
		/** `sizes` attribute for the srcset (CSS width the image is shown at). */
		sizes?: string;
		/** Largest variant offered in the srcset. */
		maxWidth?: number;
		/** Placeholder text when no image or art is available. */
		placeholder?: { title: string; year?: number | string; icon?: IconName };
		/** Load immediately (above the fold). */
		eager?: boolean;
		/** CSS object-position, e.g. "50% 20%". */
		position?: string;
	}

	let { image, src, alt = '', color, art, ratio = '2 / 3', fill = false, sizes = '200px', maxWidth = 1280, placeholder, eager = false, position }: Props = $props();

	const numericRatio = $derived.by(() => {
		const [a, b] = ratio.split('/').map((s) => parseFloat(s));
		return a && b ? a / b : 1;
	});
	const url = $derived(image ? imageUrl(image, 342) : src);
	const srcset = $derived(image ? imageSrcset(image, 92, maxWidth) : undefined);
	const blur = $derived(image?.blurhash ? blurhashToDataURL(image.blurhash, numericRatio) : undefined);
	const ground = $derived(image?.dominant ?? color ?? 'var(--surface)');
	const layout = $derived(numericRatio > 1.2 ? 'wide' : numericRatio > 0.85 ? 'square' : 'poster');

	let loaded = $state(false);
	let failed = $state(false);
	let instant = $state(false);
	let imgEl = $state<HTMLImageElement>();

	// Reset when the image changes; a cached image is shown at once, without a fade.
	$effect(() => {
		void url;
		loaded = false;
		failed = false;
		instant = false;
	});

	$effect(() => {
		if (imgEl?.complete && imgEl.naturalWidth > 0) {
			instant = true;
			loaded = true;
		}
	});

	// Generated art has no network delay; still reveal it with the same fade, on the next frame.
	$effect(() => {
		if (url || !art) return;
		const raf = requestAnimationFrame(() => (loaded = true));
		return () => cancelAnimationFrame(raf);
	});
</script>

<div class="artwork" class:fill style:aspect-ratio={fill ? undefined : ratio} style:background-color={ground}>
	{#if url && failed && placeholder}
		<TitleArt title={placeholder.title} year={placeholder.year} icon={placeholder.icon} {layout} />
	{:else if url}
		{#if blur}<img class="blur" src={blur} alt="" aria-hidden="true" />{/if}
		{#if !failed}
			<img
				bind:this={imgEl}
				class="real"
				class:loaded
				class:instant
				src={url}
				{srcset}
				sizes={srcset ? sizes : undefined}
				{alt}
				loading={eager ? 'eager' : 'lazy'}
				decoding="async"
				fetchpriority={eager ? 'high' : undefined}
				style:object-position={position}
				onload={() => (loaded = true)}
				onerror={() => (failed = true)}
			/>
		{/if}
	{:else if art}
		<div class="layer" class:loaded>{@render art()}</div>
	{:else if blur}
		<img class="blur" src={blur} alt="" aria-hidden="true" />
	{:else if placeholder}
		<TitleArt title={placeholder.title} year={placeholder.year} icon={placeholder.icon} {layout} />
	{/if}
</div>

<style>
	.artwork {
		position: relative;
		width: 100%;
		overflow: hidden;
	}

	.fill {
		position: absolute;
		inset: 0;
		height: 100%;
	}

	img,
	.layer {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.blur {
		/* The 16 px decode is upscaled; a light blur hides the bilinear seams. */
		filter: blur(6px);
		transform: scale(1.08);
	}

	.real,
	.layer {
		opacity: 0;
		transition: opacity var(--dur-slow) var(--ease);
	}

	.loaded {
		opacity: 1;
	}

	.instant {
		transition: none;
	}
</style>
