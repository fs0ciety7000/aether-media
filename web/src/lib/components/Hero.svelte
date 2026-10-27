<script lang="ts" module>
	export interface HeroItem {
		id: string;
		title: string;
		/** Micro-caps line, e.g. "Film · 2019 · 2 h 12". */
		meta: string;
		tagline?: string;
		synopsis?: string;
		/** Backdrop image; otherwise the `backdrop` snippet or the dominant colour. */
		src?: string;
		/** Title logo image (replaces the serif title when present). */
		logo?: string;
		color?: string;
		href?: string;
	}
</script>

<script lang="ts" generics="T extends HeroItem">
	import type { Snippet } from 'svelte';
	import { shell } from '$lib/shell.svelte';
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';

	interface Props {
		items: T[];
		backdrop?: Snippet<[T]>;
		/** Rotation interval in ms. */
		interval?: number;
		onplay?: (item: T) => void;
		secondaryLabel?: string;
		onsecondary?: (item: T) => void;
	}

	let { items, backdrop, interval = 8000, onplay, secondaryLabel = 'Plus d’infos', onsecondary }: Props = $props();

	let index = $state(0);
	let progress = $state(0);
	let userPaused = $state(false);
	let hovering = $state(false);
	let focusWithin = $state(false);

	const paused = $derived(userPaused || hovering || focusWithin || items.length < 2);
	const current = $derived(items[index]);

	// The header switches to on-image ink while it floats over the hero.
	$effect(() => {
		shell.overImage = true;
		return () => (shell.overImage = false);
	});

	// Time-based rotation; pausing freezes progress exactly where it is.
	$effect(() => {
		if (paused) return;
		let last = performance.now();
		let raf = requestAnimationFrame(function tick(now) {
			progress += Math.min(now - last, 100) / interval;
			last = now;
			if (progress >= 1) go(index + 1);
			raf = requestAnimationFrame(tick);
		});
		return () => cancelAnimationFrame(raf);
	});

	function go(i: number) {
		index = (i + items.length) % items.length;
		progress = 0;
	}
</script>

<section
	class="hero"
	aria-roledescription="carrousel"
	aria-label="À l’affiche"
	onmouseenter={() => (hovering = true)}
	onmouseleave={() => (hovering = false)}
	onfocusin={() => (focusWithin = true)}
	onfocusout={() => (focusWithin = false)}
>
	<div class="slides">
		{#each items as item, i (item.id)}
			<div class="slide" class:active={i === index} style:background-color={item.color} aria-hidden={i !== index}>
				{#if item.src}
					<img src={item.src} alt="" />
				{:else if backdrop}
					{@render backdrop(item)}
				{/if}
			</div>
		{/each}
		<div class="scrim" aria-hidden="true"></div>
	</div>

	<div class="content">
		{#key current.id}
			<div class="copy" aria-live={paused ? 'polite' : 'off'}>
				<p class="meta micro">{current.meta}</p>
				{#if current.logo}
					<h1 class="logo"><img src={current.logo} alt={current.title} /></h1>
				{:else}
					<h1 class="title">{current.title}</h1>
				{/if}
				{#if current.tagline}<p class="tagline">{current.tagline}</p>{/if}
				{#if current.synopsis}<p class="synopsis">{current.synopsis}</p>{/if}
			</div>
		{/key}

		<div class="actions">
			<Button variant="primary" size="lg" icon="play" iconFilled onclick={() => onplay?.(current)}>Lecture</Button>
			<Button variant="secondary" size="lg" href={current.href} onclick={() => onsecondary?.(current)}>{secondaryLabel}</Button>
		</div>
	</div>

	{#if items.length > 1}
		<div class="controls">
			<div class="dots">
				{#each items as item, i (item.id)}
					<button
						type="button"
						class="dot"
						class:active={i === index}
						aria-label="Afficher {item.title}"
						aria-current={i === index ? 'true' : undefined}
						onclick={() => go(i)}
					>
						<span style:transform={i === index ? `scaleX(${progress})` : undefined}></span>
					</button>
				{/each}
			</div>
			<button
				type="button"
				class="pause"
				aria-label={userPaused ? 'Reprendre la rotation' : 'Mettre la rotation en pause'}
				aria-pressed={userPaused}
				onclick={() => (userPaused = !userPaused)}
			>
				<Icon name={userPaused ? 'play' : 'pause'} size={14} />
			</button>
		</div>
	{/if}
</section>

<style>
	.hero {
		position: relative;
		height: clamp(540px, 84vh, 880px);
		height: clamp(540px, 84svh, 880px);
		display: flex;
		flex-direction: column;
		justify-content: flex-end;
		isolation: isolate;
		overflow: hidden;
		/* Ink is constant over imagery, whatever the theme. */
		--text: var(--on-image);
		--text-muted: var(--on-image-muted);
		color: var(--on-image);
	}

	.slides {
		position: absolute;
		inset: 0;
		z-index: -1;
		background: var(--bg-sunken);
	}

	.slide {
		position: absolute;
		inset: 0;
		opacity: 0;
		transition: opacity calc(var(--dur-slow) * 2.5) var(--ease);
	}

	.slide.active {
		opacity: 1;
	}

	.slide img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	/* A very slow drift on the active backdrop. */
	@media (prefers-reduced-motion: no-preference) {
		.slide {
			transform: scale(1.04);
			transition:
				opacity calc(var(--dur-slow) * 2.5) var(--ease),
				transform 0s linear calc(var(--dur-slow) * 2.5);
		}
		.slide.active {
			transform: scale(1);
			transition:
				opacity calc(var(--dur-slow) * 2.5) var(--ease),
				transform 12s linear;
		}
	}

	.scrim {
		position: absolute;
		inset: 0;
		background: var(--scrim), var(--scrim-side);
	}

	.scrim::before {
		content: '';
		position: absolute;
		inset: 0 0 auto;
		height: 160px;
		background: linear-gradient(to bottom, rgba(12, 11, 10, 0.45), transparent);
	}

	.content {
		padding: 0 var(--gutter) var(--sp-8);
		max-width: calc(680px + 2 * var(--gutter));
	}

	.copy {
		animation: rise var(--dur-slow) var(--ease) both;
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		@keyframes rise {
			from {
				opacity: 0;
			}
		}
	}

	.meta {
		color: var(--on-image-muted);
		margin-bottom: var(--sp-4);
	}

	.title {
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: 500;
		letter-spacing: -0.015em;
		color: var(--on-image);
	}

	.logo img {
		max-height: 140px;
		max-width: min(100%, 460px);
	}

	.tagline {
		margin-top: var(--sp-3);
		font-family: var(--font-display);
		font-style: italic;
		font-size: 1.375rem;
		line-height: 1.35;
		color: var(--on-image);
		opacity: 0.88;
	}

	.synopsis {
		margin-top: var(--sp-4);
		max-width: 54ch;
		color: var(--on-image-muted);
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-3);
		margin-top: var(--sp-6);
	}

	/* Secondary action over imagery: matte, dark, no blur. */
	.actions :global(.btn.secondary) {
		background: rgba(18, 16, 16, 0.52);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(239, 232, 220, 0.16);
	}

	.actions :global(.btn.secondary:hover) {
		background: rgba(18, 16, 16, 0.7);
	}

	.controls {
		position: absolute;
		right: var(--gutter);
		bottom: var(--sp-8);
		display: flex;
		align-items: center;
		gap: var(--sp-4);
	}

	.dots {
		display: flex;
		gap: var(--sp-2);
	}

	.dot {
		position: relative;
		width: 28px;
		height: 24px;
	}

	.dot::before,
	.dot span {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		top: 11px;
		height: 2px;
		border-radius: 1px;
	}

	.dot::before {
		background: rgba(239, 232, 220, 0.28);
	}

	.dot span {
		background: var(--on-image);
		transform: scaleX(0);
		transform-origin: left;
	}


	.pause {
		display: grid;
		place-items: center;
		width: 32px;
		height: 32px;
		border-radius: var(--r-full);
		color: var(--on-image-muted);
		box-shadow: inset 0 0 0 1px rgba(239, 232, 220, 0.22);
		transition: color var(--dur-fast) var(--ease);
	}

	.pause:hover {
		color: var(--on-image);
	}

	@media (max-width: 767px) {
		.hero {
			height: auto;
			min-height: min(78svh, 680px);
		}

		.content {
			padding-bottom: var(--sp-7);
			padding-top: calc(var(--header-h) + 30svh);
		}

		.synopsis {
			-webkit-line-clamp: 2;
			line-clamp: 2;
		}

		.tagline {
			font-size: 1.125rem;
		}

		.actions :global(.btn) {
			flex: 1;
		}

		.controls {
			position: static;
			padding: 0 var(--gutter) var(--sp-5);
			justify-content: space-between;
			margin-top: calc(-1 * var(--sp-4));
		}
	}
</style>
