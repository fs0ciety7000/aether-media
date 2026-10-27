<script lang="ts">
	import type { Snippet } from 'svelte';
	import ProgressLine from './ProgressLine.svelte';
	import Artwork from './Artwork.svelte';

	interface Props {
		title: string;
		year?: number | string;
		href?: string;
		/** Image URL; fades in over the dominant colour once loaded. */
		src?: string;
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

	let { title, year, href, src, color, art, progress, class: className, 'data-demo': demo }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class={['poster-card', className]} data-demo={demo}>
	<div class="frame">
		<Artwork {src} {color} {art} ratio="2 / 3" />
		{#if progress}<ProgressLine value={progress} />{/if}
	</div>
	<div class="meta">
		<span class="title">{title}</span>
		{#if year}<span class="year tabular">{year}</span>{/if}
	</div>
</svelte:element>

<style>
	.poster-card {
		display: block;
		width: 100%;
		border-radius: var(--r-md);
		color: var(--text-muted);
	}

	.poster-card:focus-visible {
		outline-offset: 4px;
	}

	.frame {
		position: relative;
		border-radius: var(--r-md);
		overflow: hidden;
		box-shadow: var(--shadow-1);
		transition:
			box-shadow var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	a.poster-card:hover .frame,
	.poster-card[data-demo='hover'] .frame {
		box-shadow: var(--shadow-2);
		transform: scale(1.02);
	}

	.meta {
		display: grid;
		gap: 1px;
		padding: var(--sp-3) 2px 0;
		font-size: var(--fs-small);
		line-height: var(--lh-small);
	}

	.title {
		font-weight: 500;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		transition: color var(--dur-fast) var(--ease);
	}

	a.poster-card:hover .title {
		color: var(--text);
	}

	.year {
		font-size: var(--fs-micro);
		letter-spacing: 0.02em;
	}
</style>
