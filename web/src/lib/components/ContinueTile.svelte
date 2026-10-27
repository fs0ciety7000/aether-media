<script lang="ts">
	// "Reprendre" tile: artwork filling the tile, elapsed / total time and a thin progress bar.
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import ProgressLine from './ProgressLine.svelte';

	interface Props {
		title: string;
		/** Secondary line, e.g. "Les Veilleurs · S2 · É4". */
		subtitle?: string;
		/** Formatted elapsed time, e.g. "25:00". */
		elapsed: string;
		/** Formatted total duration, e.g. "1:29:46". */
		total: string;
		/** 0–1 */
		progress: number;
		href?: string;
		src?: string;
		color?: string;
		art?: Snippet;
		/** Large tile: title at the top-left, bigger type. */
		featured?: boolean;
		class?: string;
		'data-demo'?: string;
	}

	let { title, subtitle, elapsed, total, progress, href, src, color, art, featured = false, class: className, 'data-demo': demo }: Props = $props();
</script>

<svelte:element
	this={href ? 'a' : 'div'}
	{href}
	class={['continue-tile', { featured }, className]}
	style:background-color={color ?? 'var(--surface-raised)'}
	aria-label={href ? `Reprendre ${title}${subtitle ? ` (${subtitle})` : ''}, ${elapsed} sur ${total}` : undefined}
	data-demo={demo}
>
	<span class="media" aria-hidden="true">
		{#if src}<img {src} alt="" loading="lazy" />{:else if art}{@render art()}{/if}
	</span>

	{#if featured}
		<span class="head">
			<span class="title">{title}</span>
			{#if subtitle}<span class="subtitle">{subtitle}</span>{/if}
		</span>
	{/if}

	<span class="play" aria-hidden="true"><Icon name="play" size={featured ? 22 : 18} filled /></span>

	<span class="foot">
		{#if !featured}<span class="title">{title}</span>{/if}
		<span class="times tabular"><span>{elapsed}</span><span>{total}</span></span>
		<ProgressLine value={progress} variant="inline" />
	</span>
</svelte:element>

<style>
	.continue-tile {
		position: relative;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		height: 100%;
		min-height: 0;
		padding: var(--sp-3) var(--sp-3) 12px;
		border-radius: var(--r-md);
		overflow: hidden;
		isolation: isolate;
		color: var(--on-image);
		box-shadow: var(--shadow-1), inset 0 0 0 1px var(--line);
		transition:
			transform var(--dur-base) var(--ease),
			box-shadow var(--dur-base) var(--ease);
	}

	.featured {
		padding: var(--sp-4) var(--sp-4) var(--sp-4);
		border-radius: var(--r-lg);
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
	}

	/* Legibility: deepen the bottom (times) and, for the featured tile, the top (title). */
	.media::after {
		content: '';
		position: absolute;
		inset: 0;
		background: linear-gradient(to top, rgba(2, 10, 16, 0.86) 0%, rgba(2, 10, 16, 0.35) 34%, rgba(2, 10, 16, 0) 60%);
	}

	.featured .media::after {
		background:
			linear-gradient(to bottom, rgba(2, 10, 16, 0.6) 0%, rgba(2, 10, 16, 0) 30%),
			linear-gradient(to top, rgba(2, 10, 16, 0.86) 0%, rgba(2, 10, 16, 0.3) 26%, rgba(2, 10, 16, 0) 50%);
	}

	a.continue-tile:hover,
	.continue-tile[data-demo='hover'] {
		box-shadow: var(--shadow-2), inset 0 0 0 1px var(--line-strong);
	}

	a.continue-tile:hover .media,
	.continue-tile[data-demo='hover'] .media {
		transform: scale(1.04);
	}

	.head {
		display: grid;
		gap: 2px;
		max-width: 85%;
	}

	.head .title {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
		font-weight: var(--fw-heavy);
		letter-spacing: -0.01em;
		white-space: normal;
		text-shadow: 0 1px 10px rgba(0, 0, 0, 0.35);
	}

	.subtitle {
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--on-image-muted);
	}

	.play {
		position: absolute;
		left: 50%;
		top: 50%;
		display: grid;
		place-items: center;
		width: 44px;
		height: 44px;
		margin: -22px 0 0 -22px;
		padding-left: 2px;
		border-radius: var(--r-full);
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-2);
		opacity: 0;
		transform: scale(0.92);
		transition:
			opacity var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	.featured .play {
		width: 56px;
		height: 56px;
		margin: -28px 0 0 -28px;
	}

	a.continue-tile:hover .play,
	a.continue-tile:focus-visible .play,
	.continue-tile[data-demo='hover'] .play {
		opacity: 1;
		transform: none;
	}

	.foot {
		display: grid;
		gap: 6px;
		margin-top: auto;
	}

	.title {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		line-height: 1.3;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.times {
		display: flex;
		justify-content: space-between;
		gap: var(--sp-2);
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		color: var(--on-image-muted);
	}

	.featured .times {
		font-size: var(--fs-small);
	}
</style>
