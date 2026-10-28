<script lang="ts">
	// Detail page header: backdrop with scrim, artwork, breadcrumbs, title/logo, metadata,
	// badges, tagline, overview, genres and actions.
	import type { Snippet } from 'svelte';
	import { imageUrl } from '$lib/api/images';
	import type { Image } from '$lib/api/types';
	import Artwork from '../Artwork.svelte';
	import Breadcrumbs from '../Breadcrumbs.svelte';
	import type { IconName } from '../Icon.svelte';

	interface Props {
		title: string;
		/** Small line above the title (e.g. "Film", "S1 · É3"). */
		eyebrow?: string;
		backdrop?: Image;
		poster?: Image;
		logo?: Image;
		/** poster 2:3 · square 1:1 (albums) · round (artists) · none (episodes: the backdrop is the still). */
		artwork?: 'poster' | 'square' | 'round' | 'none';
		icon?: IconName;
		year?: number;
		crumbs?: { label: string; href?: string }[];
		meta?: Snippet;
		tagline?: string;
		overview?: string;
		chips?: Snippet;
		actions?: Snippet;
	}

	let { title, eyebrow, backdrop, poster, logo, artwork = 'poster', icon, year, crumbs, meta, tagline, overview, chips, actions }: Props = $props();

	let expanded = $state(false);
	let clamped = $state(false);
	let overviewEl = $state<HTMLElement>();

	$effect(() => {
		void overview;
		expanded = false;
		if (overviewEl) clamped = overviewEl.scrollHeight > overviewEl.clientHeight + 2;
	});

	const ground = $derived(backdrop?.dominant ?? poster?.dominant);
	const logoUrl = $derived(imageUrl(logo, 500));
</script>

<header class="detail-header" class:no-art={artwork === 'none'} class:no-backdrop={!backdrop} style:--ground={ground ?? 'var(--surface-active)'}>
	<div class="backdrop" aria-hidden="true">
		{#if backdrop}
			<Artwork image={backdrop} fill sizes="100vw" maxWidth={1920} eager position="50% 25%" />
		{:else}
			<div class="fallback">
				{#if poster}<img src={imageUrl(poster, 342)} alt="" />{/if}
			</div>
		{/if}
		<div class="scrim"></div>
	</div>

	<div class="inner">
		{#if artwork !== 'none'}
			<div class="art {artwork}">
				<Artwork
					image={poster}
					ratio={artwork === 'poster' ? '2 / 3' : '1 / 1'}
					sizes="(max-width: 767px) 132px, 240px"
					eager
					placeholder={{ title, year, icon }}
					alt={`Affiche de ${title}`}
				/>
			</div>
		{/if}

		<div class="info">
			{#if crumbs?.length}<Breadcrumbs items={crumbs} />{/if}
			{#if eyebrow}<p class="eyebrow micro">{eyebrow}</p>{/if}
			{#if logoUrl}
				<h1 class="logo"><img src={logoUrl} alt={title} /></h1>
			{:else}
				<h1 class="title">{title}</h1>
			{/if}
			{#if meta}<div class="meta">{@render meta()}</div>{/if}
			{#if tagline}<p class="tagline">{tagline}</p>{/if}
			{#if overview}
				<div class="overview-wrap">
					<p class="overview" class:expanded bind:this={overviewEl}>{overview}</p>
					{#if clamped || expanded}
						<button type="button" class="more" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
							{expanded ? 'Réduire' : 'Lire la suite'}
						</button>
					{/if}
				</div>
			{/if}
			{#if chips}<div class="chips">{@render chips()}</div>{/if}
			{#if actions}<div class="actions">{@render actions()}</div>{/if}
		</div>
	</div>
</header>

<style>
	/* A card, like the home hero: imagery framed by the dashboard, not bleeding under the shell. */
	.detail-header {
		position: relative;
		isolation: isolate;
		display: flex;
		align-items: flex-end;
		margin: var(--sp-1) var(--gutter) 0;
		padding: clamp(160px, 15vw, 250px) 28px 28px;
		min-height: clamp(440px, 37vw, 580px);
		border-radius: var(--r-xl);
		overflow: hidden;
		background: var(--bg-sunken);
		box-shadow: var(--shadow-2), inset 0 0 0 1px var(--line);
		color: var(--text);
	}

	.no-backdrop {
		min-height: 0;
		padding-top: var(--sp-7);
	}

	/* Over the backdrop the ink stays light, whatever the theme (on phones the text sits below it). */
	@media (min-width: 768px) {
		.detail-header {
			--text: var(--on-image);
			--text-muted: var(--on-image-muted);
		}
	}

	.backdrop {
		position: absolute;
		inset: 0;
		z-index: -1;
		overflow: hidden;
		background: var(--ground);
	}

	.fallback {
		position: absolute;
		inset: 0;
		background:
			radial-gradient(70% 90% at 80% 20%, color-mix(in srgb, var(--ground) 80%, white 8%), transparent 70%),
			linear-gradient(160deg, color-mix(in srgb, var(--ground) 70%, var(--bg)), var(--bg));
	}

	.fallback img {
		position: absolute;
		inset: -10%;
		width: 120%;
		height: 120%;
		object-fit: cover;
		filter: blur(48px) saturate(1.1);
		opacity: 0.55;
	}

	/* Bottom and left scrims keep the text legible on any image. */
	.scrim {
		position: absolute;
		inset: 0;
		background:
			linear-gradient(to top, rgba(2, 10, 16, 0.92) 0%, rgba(2, 10, 16, 0.6) 32%, rgba(2, 10, 16, 0.12) 62%, transparent 80%),
			linear-gradient(to right, rgba(2, 10, 16, 0.7) 0%, rgba(2, 10, 16, 0.32) 42%, transparent 70%);
	}

	.inner {
		display: flex;
		align-items: flex-end;
		gap: clamp(var(--sp-5), 3vw, 40px);
		width: 100%;
		max-width: 1480px;
	}

	.art {
		flex: none;
		width: clamp(170px, 15vw, 240px);
		border-radius: var(--r-lg);
		overflow: hidden;
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	.art.square,
	.art.round {
		width: clamp(190px, 18vw, 280px);
	}

	.art.round {
		border-radius: 50%;
	}

	.info {
		display: grid;
		gap: var(--sp-3);
		min-width: 0;
		max-width: 760px;
		padding-bottom: var(--sp-1);
	}

	.eyebrow {
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.title {
		font-size: clamp(2rem, 1.2rem + 2.6vw, 3.25rem);
		line-height: 1.06;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.025em;
		text-shadow: 0 2px 24px rgba(0, 0, 0, 0.3);
	}

	.logo img {
		max-height: 130px;
		max-width: min(100%, 460px);
		object-fit: contain;
		object-position: left bottom;
		filter: drop-shadow(0 2px 16px rgba(0, 0, 0, 0.35));
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-2) var(--sp-3);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	.meta :global(.dot + .dot)::before {
		content: '·';
		margin-right: var(--sp-3);
	}

	.tagline {
		font-size: 1rem;
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.overview {
		max-width: 68ch;
		color: var(--text-muted);
		font-size: 0.9375rem;
		line-height: 1.6;
		display: -webkit-box;
		-webkit-line-clamp: 4;
		line-clamp: 4;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.overview.expanded {
		display: block;
	}

	.more {
		margin-top: var(--sp-1);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text);
		text-decoration: underline;
		text-underline-offset: 3px;
		border-radius: var(--r-xs);
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 6px;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}

	@media (min-width: 768px) {
		.actions :global(.btn.secondary) {
			background: rgba(4, 14, 22, 0.6);
			color: var(--on-image);
			box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
		}
	}

	.no-art .info {
		max-width: 820px;
	}

	@media (max-width: 767px) {
		.detail-header {
			display: block;
			min-height: 0;
			padding: 0 var(--sp-4) var(--sp-5);
			border-radius: var(--r-lg);
			background: var(--tint), var(--surface);
		}

		.backdrop {
			bottom: auto;
			height: min(56vw, 260px);
		}

		.scrim {
			background: linear-gradient(to top, var(--surface) 0%, color-mix(in srgb, var(--surface) 35%, transparent) 50%, transparent 80%);
		}

		.inner {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--sp-4);
			padding-top: min(30vw, 140px);
		}

		.no-art .inner {
			padding-top: min(50vw, 236px);
		}

		.art {
			width: 132px;
		}

		.art.square,
		.art.round {
			width: 150px;
		}

		.info {
			gap: var(--sp-2);
		}

		.actions :global(.btn.primary) {
			flex: 1;
		}
	}
</style>
