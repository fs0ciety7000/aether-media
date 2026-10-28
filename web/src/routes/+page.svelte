<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { playerHref, resolvePlayTarget } from '$lib/player/target';
	import { api, errorMessage, isAbort } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import { firstImage, imageUrl } from '$lib/api/images';
	import { libraries } from '$lib/api/stores.svelte';
	import type { HomeData, Item } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import CategoriesPanel from '$lib/components/CategoriesPanel.svelte';
	import CategoryTile from '$lib/components/CategoryTile.svelte';
	import ContinueBento from '$lib/components/ContinueBento.svelte';
	import ContinueTile from '$lib/components/ContinueTile.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Hero, { type HeroItem } from '$lib/components/Hero.svelte';
	import Onboarding from '$lib/components/home/Onboarding.svelte';
	import ItemCard from '$lib/components/ItemCard.svelte';
	import Row from '$lib/components/Row.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import { formatClock, formatRuntime, metaLine } from '$lib/format';
	import { episodeCode, isOffline, KIND_LABEL, OFFLINE_HINT } from '$lib/media';
	import { toasts } from '$lib/toast.svelte';
	import ThumbCard from '$lib/components/ThumbCard.svelte';
	import { progressOf, resumeMs } from '$lib/player/resume';
	import { seriesTitles } from '$lib/series.svelte';
	import { session } from '$lib/session.svelte';

	let home = $state<HomeData | null>(null);
	let error = $state<unknown>(null);
	let loading = $state(true);

	const empty = $derived(libraries.loaded && !libraries.error && libraries.list.length === 0);

	async function load(silent = false) {
		if (!silent) loading = true;
		try {
			home = await api.home();
			seriesTitles.resolve([...home.continueWatching, ...home.nextUp]);
			error = null;
		} catch (err) {
			if (!isAbort(err) && !silent) error = err;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		// New items or artwork: refresh quietly once the burst is over.
		const off = events.debounced(['library.scan.finished', 'item.updated', 'item.image', 'source.status'], () => load(true), 1500);
		// Back from the player in another tab / after a while: progress moved on.
		const onVisible = () => document.visibilityState === 'visible' && load(true);
		document.addEventListener('visibilitychange', onVisible);
		return () => {
			off();
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	/** "Les Veilleurs" for an episode (its series), the title otherwise. */
	const tileTitle = (it: Item) => (it.kind === 'episode' ? (seriesTitles.get(it) ?? it.title) : it.title);
	const tileSubtitle = (it: Item) => {
		if (it.kind !== 'episode') return metaLine(label(it), it.year);
		const code = episodeCode(it);
		return seriesTitles.get(it) ? metaLine(code, it.title) : code;
	};

	async function playHero(entry: { item: Item }) {
		try {
			const target = await resolvePlayTarget(entry.item);
			if (target) goto(playerHref(target));
			else toasts.show('Rien à lire pour l’instant', { detail: 'Aucun épisode n’a encore été trouvé.' });
		} catch (err) {
			toasts.show('Lecture impossible', { detail: errorMessage(err) });
		}
	}

	const label = (it: Item) => KIND_LABEL[it.kind].one.replace(/^./, (c) => c.toUpperCase());

	type HeroEntry = HeroItem & { item: Item };
	const heroItems = $derived.by<HeroEntry[]>(() => {
		if (!home) return [];
		// Hero items normally carry a backdrop; fall back to recommendations otherwise.
		const source = home.hero.length ? home.hero : home.recommended.slice(0, 5);
		return source.map((it, i) => ({
			item: it,
			id: String(it.id),
			title: it.title,
			tags: [label(it), it.video?.resolution, it.video?.hdr, isOffline(it) ? 'Hors ligne' : undefined].filter(Boolean) as string[],
			badge: i === 0 ? 'Nouveauté' : undefined,
			meta: metaLine(it.year, formatRuntime(it.runtimeMs), it.officialRating),
			synopsis: it.tagline && !it.overview ? it.tagline : it.overview,
			image: firstImage(it, 'backdrop'),
			logo: imageUrl(firstImage(it, 'logo'), 420),
			href: `/item/${it.id}`
		}));
	});
</script>

<svelte:head>
	<title>Accueil — Aether</title>
</svelte:head>

{#snippet posterFallback(entry: HeroEntry)}
	{@const poster = firstImage(entry.item, 'poster')}
	<div class="hero-fallback" style:--ground={poster?.dominant ?? 'var(--surface-active)'}>
		{#if poster}<img src={imageUrl(poster, 500)} alt="" />{/if}
	</div>
{/snippet}

<div class="home">
	{#if empty}
		<Onboarding admin={session.isAdmin} />
	{:else if loading && !home}
		<h1 class="visually-hidden">Accueil</h1>
		<div class="top" aria-busy="true">
			<Skeleton height="100%" radius="lg" class="sk-hero" />
			<Skeleton height="100%" radius="lg" class="sk-cats" />
		</div>
		<div class="sk-row" aria-hidden="true">
			<Skeleton width="220px" height="22px" radius="sm" />
			<div class="sk-posters">
				{#each Array.from({ length: 7 }, (_, i) => i) as i (i)}<Skeleton ratio="2 / 3" radius="lg" />{/each}
			</div>
		</div>
	{:else if error && !home}
		<h1 class="visually-hidden">Accueil</h1>
		<EmptyState icon="alert" title="L’accueil n’a pas pu être chargé" role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}<Button icon="refresh" onclick={() => load()}>Réessayer</Button>{/snippet}
		</EmptyState>
	{:else if home}
		<h1 class="visually-hidden">Accueil</h1>
		{#if heroItems.length || home.categories.length}
			<div class="top" class:solo={!home.categories.length} class:no-hero={!heroItems.length}>
				{#if heroItems.length}
					<Hero
						class="hero-card"
						items={heroItems}
						backdrop={posterFallback}
						onplay={playHero}
						playHint={(e) => (isOffline(e.item) ? OFFLINE_HINT : undefined)}
					/>
				{/if}
				{#if home.categories.length}
					<CategoriesPanel items={home.categories.slice(0, 8)} key={(c) => c.genre}>
						{#snippet tile(c)}
							<CategoryTile label={c.genre} href="/parcourir?genre={encodeURIComponent(c.genre)}" image={c.image} />
						{/snippet}
					</CategoriesPanel>
				{/if}
			</div>
		{/if}

		{#if home.continueWatching.length}
			<ContinueBento items={home.continueWatching} key={(it) => it.id}>
				{#snippet tile(it, { featured })}
					{@const img = firstImage(it, 'thumb', 'backdrop')}
					{@const pos = resumeMs(it) || (it.userData?.positionMs ?? 0)}
					<ContinueTile
						href={playerHref(it.id, { t: pos / 1000 })}
						title={tileTitle(it)}
						subtitle={tileSubtitle(it)}
						elapsed={formatClock(pos / 1000)}
						total={formatClock((it.runtimeMs ?? 0) / 1000)}
						progress={progressOf(it) ?? 0}
						src={imageUrl(img, featured ? 780 : 500)}
						color={img?.dominant}
						{featured}
					/>
				{/snippet}
			</ContinueBento>
		{/if}

		{#if home.nextUp.length}
			<Row title="À suivre" items={home.nextUp} key={(it) => it.id}>
				{#snippet item(ep)}
					<ThumbCard
						href={playerHref(ep.id)}
						title={seriesTitles.get(ep) ?? ep.title}
						eyebrow={seriesTitles.get(ep) ? metaLine(episodeCode(ep), ep.title) : episodeCode(ep)}
						detail={formatRuntime(ep.runtimeMs)}
						image={firstImage(ep, 'thumb', 'backdrop')}
						offline={isOffline(ep)}
					/>
				{/snippet}
			</Row>
		{/if}

		{#each home.recentlyAdded.filter((r) => r.items.length) as group (group.library.id)}
			<Row
				title="Récemment ajoutés · {group.library.name}"
				href="/bibliotheque/{group.library.id}?sort=dateAdded"
				items={group.items}
				key={(it) => it.id}
			>
				{#snippet item(it)}<ItemCard item={it} />{/snippet}
			</Row>
		{/each}

		{#if home.recommended.length}
			<Row title="Vous aimerez aussi" items={home.recommended} key={(it) => it.id}>
				{#snippet item(it)}<ItemCard item={it} />{/snippet}
			</Row>
		{/if}

		{#if !heroItems.length && !home.recentlyAdded.some((r) => r.items.length) && !home.recommended.length}
			<EmptyState icon="refresh" title="Analyse en cours">
				<p>Vos bibliothèques sont prêtes ; les premiers titres apparaîtront ici dès la fin de l’analyse.</p>
				{#snippet actions()}{#if session.isAdmin}<Button href="/reglages/bibliotheques">Voir les bibliothèques</Button>{/if}{/snippet}
			</EmptyState>
		{/if}
	{/if}
</div>

<style>
	.home {
		display: grid;
		gap: 28px;
		padding: var(--sp-1) var(--gutter) var(--sp-8);
	}

	.top {
		display: grid;
		grid-template-columns: minmax(0, 1.95fr) minmax(300px, 1fr);
		gap: var(--gap);
		min-height: clamp(340px, 25.5vw, 460px);
	}

	.top.solo,
	.top.no-hero {
		grid-template-columns: minmax(0, 1fr);
	}

	.top.no-hero {
		min-height: 0;
	}

	.top.no-hero :global(.categories .grid) {
		grid-template-columns: repeat(4, minmax(0, 1fr));
		grid-auto-rows: 80px;
	}

	.hero-fallback {
		position: absolute;
		inset: 0;
		background:
			radial-gradient(90% 120% at 85% 30%, color-mix(in srgb, var(--ground) 70%, white 6%), transparent 70%),
			linear-gradient(120deg, var(--bg-sunken), color-mix(in srgb, var(--ground) 60%, var(--bg-sunken)));
	}

	.hero-fallback img {
		position: absolute;
		right: 7%;
		top: 50%;
		height: 78%;
		width: auto;
		aspect-ratio: 2 / 3;
		object-fit: cover;
		border-radius: var(--r-lg);
		transform: translateY(-50%) rotate(2deg);
		box-shadow: var(--shadow-3);
	}

	.sk-row {
		display: grid;
		gap: var(--sp-4);
	}

	.sk-posters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(clamp(140px, 12.5vw, 184px), 1fr));
		grid-template-rows: auto;
		grid-auto-rows: 0;
		overflow: hidden;
		gap: var(--gap);
	}

	@media (max-width: 1023px) {
		.top {
			grid-template-columns: minmax(0, 1fr);
		}

		.top :global(.categories .grid) {
			grid-template-columns: repeat(4, minmax(0, 1fr));
			grid-auto-rows: 72px;
		}

		.top :global(.sk-cats) {
			display: none;
		}
	}

	@media (max-width: 767px) {
		.home {
			gap: var(--sp-5);
			padding-top: 0;
		}

		.top {
			min-height: 0;
			gap: var(--sp-5);
		}

		.top :global(.sk-hero) {
			height: 400px !important;
		}

		.hero-fallback img {
			height: 52%;
			top: 34%;
			right: 50%;
			transform: translate(50%, -50%) rotate(2deg);
		}
	}
</style>
