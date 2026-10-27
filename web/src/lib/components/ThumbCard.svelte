<script lang="ts">
	import type { Snippet } from 'svelte';
	import Artwork from './Artwork.svelte';
	import Icon from './Icon.svelte';
	import ProgressLine from './ProgressLine.svelte';

	interface Props {
		title: string;
		/** Over-title in micro caps, e.g. "Les Veilleurs · S2 · É4". */
		eyebrow?: string;
		/** Right-aligned detail, e.g. "Reste 34 min". */
		detail?: string;
		href?: string;
		src?: string;
		color?: string;
		art?: Snippet;
		progress?: number;
		/** Show the play affordance on hover. */
		playable?: boolean;
		class?: string;
		/** Forces a visual state in the design gallery. */
		'data-demo'?: string;
	}

	let { title, eyebrow, detail, href, src, color, art, progress, playable = true, class: className, 'data-demo': demo }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} {href} class={['thumb-card', className]} data-demo={demo}>
	<div class="frame">
		<Artwork {src} {color} {art} ratio="16 / 9" />
		{#if playable}
			<span class="play" aria-hidden="true"><Icon name="play" size={18} filled /></span>
		{/if}
		{#if progress}<ProgressLine value={progress} />{/if}
	</div>
	<div class="meta">
		<div class="text">
			{#if eyebrow}<span class="eyebrow micro">{eyebrow}</span>{/if}
			<span class="title">{title}</span>
		</div>
		{#if detail}<span class="detail tabular">{detail}</span>{/if}
	</div>
</svelte:element>

<style>
	.thumb-card {
		display: block;
		width: 100%;
		border-radius: var(--r-md);
		color: var(--text-muted);
	}

	.thumb-card:focus-visible {
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

	a.thumb-card:hover .frame,
	.thumb-card[data-demo='hover'] .frame {
		box-shadow: var(--shadow-2);
		transform: scale(1.02);
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
		background: rgba(18, 16, 16, 0.6);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(239, 232, 220, 0.18);
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
		padding: var(--sp-3) 2px 0;
	}

	.text {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.eyebrow {
		color: var(--text-muted);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.title {
		font-size: var(--fs-small);
		line-height: var(--lh-small);
		font-weight: 500;
		color: var(--text);
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.detail {
		flex: none;
		font-size: var(--fs-micro);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}
</style>
