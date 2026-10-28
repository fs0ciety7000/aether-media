<script lang="ts">
	// Genre tile: artwork with the label centred on it.
	import type { Snippet } from 'svelte';
	import { blurhashToDataURL } from '$lib/api/blurhash';
	import { imageUrl } from '$lib/api/images';
	import type { Image } from '$lib/api/types';

	interface Props {
		label: string;
		href?: string;
		src?: string;
		image?: Image;
		color?: string;
		art?: Snippet;
		'data-demo'?: string;
	}

	let { label, href, src, image, color, art, 'data-demo': demo }: Props = $props();
	const url = $derived(image ? imageUrl(image, 240) : src);
	const blur = $derived(image?.blurhash ? blurhashToDataURL(image.blurhash, 16 / 9) : undefined);
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class="category-tile" style:background-color={image?.dominant ?? color ?? 'var(--surface-raised)'} data-demo={demo}>
	<span class="media" aria-hidden="true">
		{#if url}<img src={url} alt="" loading="lazy" style:background-image={blur ? `url(${blur})` : undefined} />{:else if art}{@render art()}{/if}
	</span>
	<span class="label">{label}</span>
</svelte:element>

<style>
	.category-tile {
		position: relative;
		display: grid;
		place-items: center;
		min-height: 64px;
		height: 100%;
		border-radius: var(--r-sm);
		overflow: hidden;
		isolation: isolate;
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--on-image);
	}

	.media {
		position: absolute;
		inset: 0;
		z-index: -1;
		transition: transform var(--dur-slow) var(--ease);
	}

	.media img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		background-size: cover;
	}

	/* Even darkening so the label always reads. */
	.media::after {
		content: '';
		position: absolute;
		inset: 0;
		background: radial-gradient(ellipse at center, rgba(2, 10, 16, 0.55), rgba(2, 10, 16, 0.3) 75%);
		transition: background-color var(--dur-base) var(--ease);
	}

	.label {
		padding: 0 var(--sp-2);
		font-size: var(--fs-body);
		font-weight: var(--fw-bold);
		text-align: center;
		text-shadow: 0 1px 8px rgba(0, 0, 0, 0.5);
	}

	a.category-tile:hover .media,
	.category-tile[data-demo='hover'] .media {
		transform: scale(1.06);
	}

	a.category-tile:hover .media::after,
	.category-tile[data-demo='hover'] .media::after {
		background-color: rgba(2, 10, 16, 0.12);
	}
</style>
