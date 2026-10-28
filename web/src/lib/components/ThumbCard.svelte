<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Image } from '$lib/api/types';
	import Artwork from './Artwork.svelte';
	import Icon from './Icon.svelte';
	import ProgressLine from './ProgressLine.svelte';

	interface Props {
		title: string;
		/** Secondary line under the title, e.g. "Les Veilleurs · S2 · É4". */
		eyebrow?: string;
		/** Right-aligned detail, e.g. "Reste 34 min". */
		detail?: string;
		href?: string;
		image?: Image;
		src?: string;
		sizes?: string;
		color?: string;
		art?: Snippet;
		progress?: number;
		/** Show the play affordance on hover. */
		playable?: boolean;
		class?: string;
		/** Forces a visual state in the design gallery. */
		'data-demo'?: string;
	}

	let { title, eyebrow, detail, href, image, src, sizes = '(max-width: 767px) 80vw, 300px', color, art, progress, playable = true, class: className, 'data-demo': demo }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class={['thumb-card', className]} data-demo={demo}>
	<div class="frame">
		<Artwork {image} {src} {color} {art} {sizes} ratio="16 / 9" placeholder={{ title }} />
		{#if playable}
			<span class="play" aria-hidden="true"><Icon name="play" size={18} filled /></span>
		{/if}
		{#if progress}<ProgressLine value={progress} />{/if}
	</div>
	<div class="meta">
		<div class="text">
			<span class="title">{title}</span>
			{#if eyebrow}<span class="eyebrow">{eyebrow}</span>{/if}
		</div>
		{#if detail}<span class="detail tabular">{detail}</span>{/if}
	</div>
</svelte:element>

<style>
	.thumb-card {
		display: block;
		width: 100%;
		border-radius: var(--r-lg);
		color: var(--text-muted);
	}

	.thumb-card:focus-visible {
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

	a.thumb-card:hover .frame,
	.thumb-card[data-demo='hover'] .frame {
		box-shadow: var(--shadow-2);
		transform: translateY(-3px);
	}

	.play {
		position: absolute;
		left: 50%;
		top: 50%;
		display: grid;
		place-items: center;
		width: 48px;
		height: 48px;
		margin: -24px 0 0 -24px;
		padding-left: 2px;
		border-radius: var(--r-full);
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-2);
		opacity: 0;
		transition: opacity var(--dur-base) var(--ease);
	}

	a.thumb-card:hover .play,
	a.thumb-card:focus-visible .play,
	.thumb-card[data-demo='hover'] .play {
		opacity: 1;
	}

	.meta {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-3);
		padding: 10px 4px 0;
	}

	.text {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.eyebrow {
		font-size: var(--fs-micro);
		color: var(--text-muted);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.title {
		font-size: var(--fs-body);
		line-height: var(--lh-small);
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.detail {
		flex: none;
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}
</style>
