<script lang="ts" module>
	export interface HeroItem {
		id: string;
		title: string;
		/** Genre tags shown as small chips at the top-left. */
		tags?: string[];
		/** Badge at the top-right, e.g. "Tendance". */
		badge?: string;
		/** Optional metadata line under the title, e.g. "2019 · 2 h 12". */
		meta?: string;
		synopsis?: string;
		/** Backdrop image; otherwise the `backdrop` snippet or the dominant colour. */
		src?: string;
		/** Title logo image (replaces the text title when present). */
		logo?: string;
		color?: string;
		href?: string;
		/** API backdrop (blurhash, srcset); preferred over `src`. */
		image?: import('$lib/api/types').Image;
		/** object-position of the backdrop on narrow screens (keeps the subject in frame). */
		mobilePosition?: string;
	}
</script>

<script lang="ts" generics="T extends HeroItem">
	import type { Snippet } from 'svelte';
	import Artwork from './Artwork.svelte';
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';
	import Tag from './Tag.svelte';

	interface Props {
		items: T[];
		backdrop?: Snippet<[T]>;
		/** Rotation interval in ms. */
		interval?: number;
		onplay?: (item: T) => void;
		/** When set, "Lecture" is shown unavailable with this explanation as tooltip. */
		playHint?: string | ((item: T) => string | undefined);
		secondaryLabel?: string;
		onsecondary?: (item: T) => void;
		class?: string;
	}

	let { items, backdrop, interval = 9000, onplay, playHint, secondaryLabel = 'Plus d’infos', onsecondary, class: className }: Props = $props();

	let index = $state(0);
	let progress = $state(0);
	let userPaused = $state(false);
	let hovering = $state(false);
	let focusWithin = $state(false);

	const paused = $derived(userPaused || hovering || focusWithin || items.length < 2);
	const current = $derived(items[index]);
	const hint = $derived(typeof playHint === 'function' ? (current ? playHint(current) : undefined) : playHint);

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
	class={['hero', className]}
	aria-roledescription="carrousel"
	aria-label="À l’affiche"
	onmouseenter={() => (hovering = true)}
	onmouseleave={() => (hovering = false)}
	onfocusin={() => (focusWithin = true)}
	onfocusout={() => (focusWithin = false)}
>
	<div class="slides">
		{#each items as item, i (item.id)}
			<div
				class="slide"
				class:active={i === index}
				style:background-color={item.image?.dominant ?? item.color}
				style:--mobile-pos={item.mobilePosition ?? '62% 50%'}
				aria-hidden={i !== index}
			>
				{#if item.image}
					<Artwork image={item.image} fill sizes="(max-width: 1023px) 100vw, 66vw" maxWidth={1920} eager={i === 0} />
				{:else if item.src}
					<img src={item.src} alt="" />
				{:else if backdrop}
					{@render backdrop(item)}
				{/if}
			</div>
		{/each}
		<div class="scrim" aria-hidden="true"></div>
	</div>

	{#key current.id}
		<div class="top">
			{#if current.tags?.length}
				<ul class="tags" role="list" aria-label="Genres">
					{#each current.tags.slice(0, 2) as tag (tag)}<li><Tag variant="image">{tag}</Tag></li>{/each}
				</ul>
			{/if}
			{#if current.badge}<span class="badge"><Tag variant="image" icon="trending">{current.badge}</Tag></span>{/if}
		</div>

		<div class="copy" aria-live={paused ? 'polite' : 'off'}>
			{#if current.logo}
				<h2 class="logo"><img src={current.logo} alt={current.title} /></h2>
			{:else}
				<h2 class="title">{current.title}</h2>
			{/if}
			{#if current.meta}<p class="meta">{current.meta}</p>{/if}
			{#if current.synopsis}<p class="synopsis">{current.synopsis}</p>{/if}
		</div>
	{/key}

	<div class="bottom">
		<div class="actions">
			<Button
				variant="primary"
				size="lg"
				icon="play"
				iconFilled
				aria-disabled={hint ? 'true' : undefined}
				title={hint}
				onclick={() => !hint && onplay?.(current)}>Lecture</Button
			>
			<Button variant="secondary" size="lg" href={current.href} onclick={() => onsecondary?.(current)}>{secondaryLabel}</Button>
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
					<Icon name={userPaused ? 'play' : 'pause'} size={13} filled={userPaused} />
				</button>
			</div>
		{/if}
	</div>
</section>

<style>
	.hero {
		position: relative;
		display: flex;
		flex-direction: column;
		min-height: 360px;
		padding: var(--sp-5) 28px 28px;
		border-radius: var(--r-xl);
		isolation: isolate;
		overflow: hidden;
		background: var(--bg-sunken);
		box-shadow: var(--shadow-2), inset 0 0 0 1px var(--line);
		/* Ink is constant over imagery, whatever the theme. */
		--text: var(--on-image);
		--text-muted: var(--on-image-muted);
		color: var(--on-image);
	}

	.slides {
		position: absolute;
		inset: 0;
		z-index: -1;
	}

	.slide {
		position: absolute;
		inset: 0;
		opacity: 0;
		transition: opacity calc(var(--dur-slow) * 2) var(--ease);
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
				opacity calc(var(--dur-slow) * 2) var(--ease),
				transform 0s linear calc(var(--dur-slow) * 2);
		}
		.slide.active {
			transform: scale(1);
			transition:
				opacity calc(var(--dur-slow) * 2) var(--ease),
				transform 14s linear;
		}
	}

	.scrim {
		position: absolute;
		inset: 0;
		background: var(--scrim-side), linear-gradient(to top, rgba(2, 10, 16, 0.55), transparent 45%);
	}

	.top {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--sp-3);
		animation: fade var(--dur-slow) var(--ease) both;
	}

	.tags {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
		list-style: none;
		padding: 0;
	}

	.badge {
		margin-left: auto;
	}

	.copy {
		max-width: min(440px, 62%);
		margin-top: auto;
		padding-top: var(--sp-6);
		animation: rise var(--dur-slow) var(--ease) both;
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(6px);
		}
	}

	@keyframes fade {
		from {
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		@keyframes rise {
			from {
				opacity: 0;
			}
		}
	}

	.title {
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: var(--fw-heavy);
		letter-spacing: -0.02em;
		color: var(--on-image);
		text-shadow: 0 2px 18px rgba(0, 0, 0, 0.25);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.logo img {
		max-height: 120px;
		max-width: min(100%, 420px);
		object-fit: contain;
		object-position: left bottom;
		filter: drop-shadow(0 2px 14px rgba(0, 0, 0, 0.35));
	}

	.meta {
		margin-top: var(--sp-2);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--on-image-muted);
	}

	.synopsis {
		margin-top: var(--sp-3);
		max-width: 46ch;
		font-size: var(--fs-body);
		line-height: var(--lh-body);
		color: var(--on-image-muted);
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.bottom {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		margin-top: var(--sp-5);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-3);
	}

	/* Secondary action over imagery: dark matte pill, no blur. */
	.actions :global(.btn.secondary) {
		background: rgba(4, 14, 22, 0.6);
		color: var(--on-image);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
	}

	.actions :global(.btn.secondary:hover) {
		background: rgba(4, 14, 22, 0.78);
	}

	.controls {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
	}

	.dots {
		display: flex;
		gap: 2px;
	}

	.dot {
		position: relative;
		width: 26px;
		height: 24px;
	}

	.dot::before,
	.dot span {
		content: '';
		position: absolute;
		left: 3px;
		right: 3px;
		top: 10.5px;
		height: 3px;
		border-radius: var(--r-full);
	}

	.dot::before {
		background: rgba(255, 255, 255, 0.28);
	}

	.dot span {
		background: var(--on-image);
		transform: scaleX(0);
		transform-origin: left;
	}

	.pause {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border-radius: var(--r-sm);
		background: rgba(4, 14, 22, 0.5);
		color: var(--on-image-muted);
		box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
		transition: color var(--dur-fast) var(--ease);
	}

	.pause:hover {
		color: var(--on-image);
	}

	@media (max-width: 767px) {
		.hero {
			min-height: 400px;
			padding: var(--sp-4) var(--sp-4) var(--sp-4);
			border-radius: var(--r-lg);
		}

		.scrim {
			background: var(--scrim);
		}

		.copy {
			max-width: none;
		}

		.title {
			font-size: 1.75rem;
		}

		.slide :global(img) {
			object-position: var(--mobile-pos);
		}

		.slide :global(svg) {
			/* Portrait crop: keep the key light in frame. */
			transform: translateX(-4%) scale(1.3);
		}

		.synopsis {
			-webkit-line-clamp: 2;
			line-clamp: 2;
		}

		.bottom {
			flex-direction: column;
			align-items: stretch;
		}

		.actions :global(.btn) {
			flex: 1;
		}

		.controls {
			justify-content: center;
		}
	}
</style>
