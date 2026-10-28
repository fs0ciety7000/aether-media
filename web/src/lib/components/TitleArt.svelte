<script lang="ts">
	// Placeholder artwork when an item has no image: its title typeset on a calm tinted tile.
	import { hueFrom } from '$lib/media';
	import Icon, { type IconName } from './Icon.svelte';

	interface Props {
		title: string;
		year?: number | string;
		icon?: IconName;
		/** poster (2:3, title low) · wide (16:9) · square (covers, artists, title centred). */
		layout?: 'poster' | 'wide' | 'square';
	}

	let { title, year, icon, layout = 'poster' }: Props = $props();
	const hue = $derived(hueFrom(title));
</script>

<div class="title-art {layout}" style:--h={hue} aria-hidden="true">
	<span class="glow"></span>
	{#if icon}<span class="icon"><Icon name={icon} size={18} /></span>{/if}
	<span class="text">
		<span class="title">{title}</span>
		{#if year}<span class="year tabular">{year}</span>{/if}
	</span>
</div>

<style>
	.title-art {
		position: absolute;
		inset: 0;
		overflow: hidden;
		container-type: inline-size;
		color: #f6ffff;
		background:
			radial-gradient(120% 80% at 18% 0%, hsl(var(--h) 38% 34% / 0.75), transparent 62%),
			linear-gradient(165deg, hsl(var(--h) 34% 21%) 0%, hsl(calc(var(--h) + 24) 38% 11%) 72%, hsl(calc(var(--h) + 30) 40% 8%) 100%);
	}

	/* A soft key light, like the generated demo art — never a glow on the UI itself. */
	.glow {
		position: absolute;
		right: -18%;
		top: 30%;
		width: 70%;
		aspect-ratio: 1;
		border-radius: 50%;
		background: radial-gradient(circle, hsl(calc(var(--h) - 20) 50% 62% / 0.22), transparent 66%);
	}

	.icon {
		position: absolute;
		left: 9cqi;
		top: 9cqi;
		display: grid;
		place-items: center;
		width: max(26px, 16cqi);
		height: max(26px, 16cqi);
		border-radius: var(--r-sm);
		background: rgba(255, 255, 255, 0.08);
		color: rgba(246, 255, 255, 0.72);
	}

	.text {
		position: absolute;
		left: 9cqi;
		right: 9cqi;
		bottom: 10cqi;
		display: grid;
		gap: 3cqi;
	}

	.title {
		font-size: max(12px, 11.5cqi);
		line-height: 1.06;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.015em;
		text-wrap: balance;
		overflow-wrap: anywhere;
		display: -webkit-box;
		-webkit-line-clamp: 4;
		line-clamp: 4;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.year {
		font-size: max(10px, 5.5cqi);
		font-weight: var(--fw-bold);
		letter-spacing: 0.12em;
		color: rgba(246, 255, 255, 0.66);
	}

	.wide .title {
		font-size: max(13px, 6.5cqi);
		-webkit-line-clamp: 2;
		line-clamp: 2;
	}

	.wide .text {
		left: 5cqi;
		right: 5cqi;
		bottom: 6cqi;
		gap: 1.5cqi;
	}

	.wide .icon {
		left: 5cqi;
		top: 6cqi;
		width: max(26px, 8cqi);
		height: max(26px, 8cqi);
	}

	.wide .year {
		font-size: max(10px, 3cqi);
	}

	.square .text {
		top: 50%;
		bottom: auto;
		transform: translateY(-50%);
		text-align: center;
		justify-items: center;
	}

	.square .title {
		font-size: max(12px, 10cqi);
	}

	.square .icon {
		display: none;
	}
</style>
