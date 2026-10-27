<script lang="ts">
	// Generated poster: the film's key-art scene, cropped to 2:3, with a typographic treatment per layout.
	import BackdropArt from './BackdropArt.svelte';
	import type { BackdropScene, PosterLayout } from './data';

	interface Props {
		title: string;
		year?: number;
		credit?: string;
		hue: number;
		hue2?: number;
		sat?: number;
		scene?: BackdropScene;
		layout?: PosterLayout;
	}

	let { title, year, credit, hue, hue2 = 232, sat, scene = 'dunes', layout = 'sun' }: Props = $props();

	const firstWord = $derived(title.replace(/^(L’|L'|Le |La |Les )/, '').split(' ')[0]);
</script>

<div class="art {layout}" style:--h={hue} style:--h2={hue2} aria-hidden="true">
	<BackdropArt {hue} {hue2} {sat} {scene} focus={0.5} />
	<span class="shade"></span>

	{#if layout === 'frame'}
		<span class="rule"></span>
		<span class="credit">{credit}</span>
		<span class="title">{title}</span>
		{#if year}<span class="year">{year}</span>{/if}
	{:else if layout === 'type'}
		<span class="giant" class:solo={firstWord === title}>{firstWord}</span>
		<span class="credit">{credit}</span>
		{#if firstWord !== title}<span class="title">{title}</span>{/if}
	{:else}
		<span class="credit">{credit}</span>
		<span class="title">{title}</span>
	{/if}
</div>

<style>
	.art {
		--ink: #f6ffff;
		position: absolute;
		inset: 0;
		overflow: hidden;
		container-type: inline-size;
		color: var(--ink);
		font-family: var(--font-ui);
	}

	/* Legibility: deepen the bottom (or top, for horizon) where the title sits. */
	.shade {
		position: absolute;
		inset: 0;
		background: linear-gradient(to top, hsl(var(--h2) 50% 5% / 0.82) 0%, hsl(var(--h2) 50% 5% / 0.25) 38%, transparent 60%);
	}

	.horizon .shade {
		background: linear-gradient(to bottom, hsl(var(--h2) 50% 5% / 0.7) 0%, transparent 42%),
			linear-gradient(to top, hsl(var(--h2) 50% 5% / 0.5) 0%, transparent 30%);
	}

	.credit {
		position: absolute;
		left: 9cqi;
		right: 9cqi;
		bottom: 8cqi;
		font-size: max(6px, 4.4cqi);
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		opacity: 0.7;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.title {
		position: absolute;
		left: 9cqi;
		right: 9cqi;
		bottom: 15cqi;
		font-size: 12.5cqi;
		line-height: 1.02;
		font-weight: 800;
		letter-spacing: -0.02em;
		text-wrap: balance;
		text-shadow: 0 2px 12px rgba(0, 0, 0, 0.35);
	}

	/* Horizon: title high in the sky. */
	.horizon .title {
		top: 11cqi;
		bottom: auto;
		font-size: 13cqi;
	}
	.horizon .credit {
		top: auto;
	}

	/* Frame: inset rule, centred capitals. */
	.frame .rule {
		position: absolute;
		inset: 5.5cqi;
		border: 1px solid rgba(246, 255, 255, 0.35);
		border-radius: 3cqi;
	}
	.frame .credit {
		top: 12cqi;
		bottom: auto;
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
		bottom: 12cqi;
		text-align: center;
		font-size: max(6px, 4.4cqi);
		font-weight: 700;
		letter-spacing: 0.2em;
		opacity: 0.75;
	}

	/* Type: one giant cropped word, tinted by the key light. */
	.type .giant {
		position: absolute;
		left: -2cqi;
		top: 8cqi;
		font-size: 46cqi;
		line-height: 0.85;
		font-weight: 900;
		letter-spacing: -0.05em;
		color: hsl(var(--h) 60% 80% / 0.16);
		white-space: nowrap;
	}
	.type .giant.solo {
		left: 8cqi;
		top: auto;
		bottom: 15cqi;
		font-size: 30cqi;
		font-weight: 800;
		color: var(--ink);
		text-shadow: 0 2px 16px rgba(0, 0, 0, 0.35);
	}

	/* Column: centred. */
	.column .title,
	.column .credit {
		text-align: center;
	}
</style>
