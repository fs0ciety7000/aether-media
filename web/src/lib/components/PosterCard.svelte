<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Image } from '$lib/api/types';
	import ProgressLine from './ProgressLine.svelte';
	import Artwork from './Artwork.svelte';
	import type { IconName } from './Icon.svelte';

	interface Props {
		title: string;
		year?: number | string;
		href?: string;
		/** API image (blurhash, dominant colour, srcset). */
		image?: Image;
		/** Image URL; fades in over the dominant colour once loaded. */
		src?: string;
		/** CSS aspect ratio of the artwork (2:3 posters, 1:1 covers). */
		ratio?: string;
		/** Rendered width, for the srcset. */
		sizes?: string;
		/** Icon on the generated placeholder when there is no image. */
		icon?: IconName;
		/** Round artwork (artists). */
		round?: boolean;
		/** Replaces the year under the title. */
		subtitle?: string;
		/** Dominant colour (from blurhash) shown while the image loads. */
		color?: string;
		/** Generated artwork, used instead of an image. */
		art?: Snippet;
		/** 0–1; shows a thin accent bar. */
		progress?: number;
		class?: string;
		/** Forces a visual state in the design gallery. */
		'data-demo'?: string;
	}

	let { title, year, href, image, src, ratio = '2 / 3', sizes = '(max-width: 767px) 45vw, 184px', icon, round = false, subtitle, color, art, progress, class: className, 'data-demo': demo }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class={['poster-card', className]} data-demo={demo}>
	<div class="frame" class:round>
		<Artwork {image} {src} {color} {art} {ratio} {sizes} placeholder={{ title, year, icon }} alt="" />
		{#if progress}<ProgressLine value={progress} />{/if}
	</div>
	<div class="meta">
		<span class="title">{title}</span>
		{#if subtitle}<span class="year">{subtitle}</span>{:else if year}<span class="year tabular">{year}</span>{/if}
	</div>
</svelte:element>

<style>
	.poster-card {
		display: block;
		width: 100%;
		border-radius: var(--r-lg);
		color: var(--text-muted);
	}

	.poster-card:focus-visible {
		outline-offset: 4px;
	}

	.frame {
		position: relative;
		border-radius: var(--r-lg);
		overflow: hidden;
		box-shadow: var(--shadow-1), inset 0 0 0 1px var(--line);
		isolation: isolate;
		transition:
			box-shadow var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	a.poster-card:hover .frame,
	.poster-card[data-demo='hover'] .frame {
		box-shadow: var(--shadow-2);
		transform: translateY(-3px);
	}

	.frame.round {
		border-radius: 50%;
	}

	.round + .meta {
		text-align: center;
	}

	.meta {
		display: grid;
		gap: 1px;
		padding: 10px 4px 0;
		font-size: var(--fs-body);
		line-height: var(--lh-small);
	}

	.title {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		transition: color var(--dur-fast) var(--ease);
	}

	.year {
		font-size: var(--fs-micro);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}
</style>
