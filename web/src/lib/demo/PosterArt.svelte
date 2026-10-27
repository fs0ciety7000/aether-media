<script lang="ts">
	// Generated minimalist poster: muted tones derived from one hue, serif title typeset on it.
	import type { PosterLayout } from './data';

	interface Props {
		title: string;
		year?: number;
		credit?: string;
		hue: number;
		sat?: number;
		layout?: PosterLayout;
	}

	let { title, year, credit, hue, sat = 18, layout = 'sun' }: Props = $props();

	const firstWord = $derived(title.replace(/^(L’|L'|Le |La |Les )/, '').split(' ')[0]);
</script>

<div class="art {layout}" style:--h={hue} style:--s="{sat}%" aria-hidden="true">
	{#if layout === 'sun'}
		<span class="disc"></span>
		<span class="credit micro">{credit}</span>
		<span class="title">{title}</span>
	{:else if layout === 'horizon'}
		<span class="sky"></span>
		<span class="sea"></span>
		<span class="credit micro">{credit}</span>
		<span class="title">{title}</span>
	{:else if layout === 'frame'}
		<span class="rule"></span>
		<span class="credit micro">{credit}</span>
		<span class="title">{title}</span>
		{#if year}<span class="year micro">{year}</span>{/if}
	{:else if layout === 'type'}
		<span class="giant" class:solo={firstWord === title}>{firstWord}</span>
		<span class="credit micro">{credit}</span>
		{#if firstWord !== title}<span class="title">{title}</span>{/if}
	{:else}
		<span class="door"></span>
		<span class="credit micro">{credit}</span>
		<span class="title">{title}</span>
	{/if}
</div>

<style>
	.art {
		--deep: hsl(var(--h) var(--s) 11%);
		--ground: hsl(var(--h) var(--s) 21%);
		--mid: hsl(var(--h) calc(var(--s) * 0.9) 34%);
		--light: hsl(var(--h) calc(var(--s) + 6%) 60%);
		--ink: hsl(var(--h) 24% 89%);
		position: absolute;
		inset: 0;
		overflow: hidden;
		container-type: inline-size;
		background: linear-gradient(170deg, var(--ground), var(--deep));
		color: var(--ink);
		font-family: var(--font-display);
	}

	.credit {
		position: absolute;
		top: 9cqi;
		left: 9cqi;
		right: 9cqi;
		font-family: var(--font-ui);
		font-size: max(6px, 4.6cqi);
		letter-spacing: 0.16em;
		opacity: 0.72;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.title {
		position: absolute;
		left: 9cqi;
		right: 9cqi;
		bottom: 10cqi;
		font-size: 13cqi;
		line-height: 1;
		font-weight: 500;
		letter-spacing: -0.01em;
		text-wrap: balance;
	}

	/* Sun: a low disc in a dusk sky. */
	.sun .disc {
		position: absolute;
		width: 62cqi;
		aspect-ratio: 1;
		left: 19cqi;
		top: 38%;
		border-radius: 50%;
		background: radial-gradient(circle at 50% 40%, var(--light), var(--mid) 70%);
		opacity: 0.55;
	}

	/* Horizon: sky over a darker sea, title in the sky. */
	.horizon .sky {
		position: absolute;
		inset: 0 0 38% 0;
		background: linear-gradient(to bottom, var(--deep), var(--mid) 85%, var(--light));
		opacity: 0.85;
	}
	.horizon .sea {
		position: absolute;
		inset: 62% 0 0 0;
		background: linear-gradient(to bottom, var(--ground), var(--deep));
		box-shadow: 0 -1px 0 hsl(var(--h) 20% 75% / 0.35);
	}
	.horizon .title {
		top: 22%;
		bottom: auto;
		font-style: italic;
		font-size: 14cqi;
	}

	/* Frame: a thin inset rule, centred capitals. */
	.frame {
		background: linear-gradient(180deg, var(--mid), var(--ground) 70%);
	}
	.frame .rule {
		position: absolute;
		inset: 6cqi;
		border: 1px solid hsl(var(--h) 20% 85% / 0.28);
	}
	.frame .credit {
		top: 13cqi;
		text-align: center;
	}
	.frame .title {
		top: 50%;
		bottom: auto;
		transform: translateY(-50%);
		text-align: center;
		font-size: 10cqi;
		line-height: 1.1;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}
	.frame .year {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 13cqi;
		text-align: center;
		font-family: var(--font-ui);
		font-size: max(6px, 4.6cqi);
		letter-spacing: 0.2em;
		opacity: 0.7;
	}

	/* Type: one giant cropped word. */
	.type .giant {
		position: absolute;
		left: -3cqi;
		bottom: 18%;
		font-size: 54cqi;
		line-height: 0.8;
		font-weight: 600;
		letter-spacing: -0.04em;
		color: hsl(var(--h) var(--s) 42%);
		opacity: 0.55;
		white-space: nowrap;
	}
	.type .giant.solo {
		left: 7cqi;
		bottom: 9cqi;
		font-size: 34cqi;
		font-weight: 500;
		color: var(--ink);
		opacity: 1;
	}
	.type .title {
		font-size: 11cqi;
	}

	/* Column: a lit doorway in a dark room. */
	.column {
		background: linear-gradient(180deg, var(--deep), var(--ground));
	}
	.column .door {
		position: absolute;
		left: 36cqi;
		width: 28cqi;
		top: 24%;
		height: 44%;
		background: linear-gradient(to bottom, var(--light), var(--mid));
		opacity: 0.6;
		box-shadow: 0 0 0 1px hsl(var(--h) 20% 80% / 0.1);
	}
	.column .door::after {
		content: '';
		position: absolute;
		left: -40cqi;
		right: -40cqi;
		top: 100%;
		height: 60%;
		background: linear-gradient(to bottom, hsl(var(--h) var(--s) 50% / 0.18), transparent);
		clip-path: polygon(40% 0, 60% 0, 100% 100%, 0 100%);
	}
	.column .title {
		text-align: center;
	}
</style>
