<script lang="ts">
	// Item detail, adapted to the kind: movie, series, season, episode, artist, album.
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, errorMessage, isAbort, normalizeUserData } from '$lib/api/client';
	import { events } from '$lib/api/events.svelte';
	import { firstImage } from '$lib/api/images';
	import { libraries } from '$lib/api/stores.svelte';
	import type { Item, ItemDetail } from '$lib/api/types';
	import Button from '$lib/components/Button.svelte';
	import CastRow from '$lib/components/detail/CastRow.svelte';
	import DetailHeader from '$lib/components/detail/DetailHeader.svelte';
	import EpisodeList from '$lib/components/detail/EpisodeList.svelte';
	import TechPanel from '$lib/components/detail/TechPanel.svelte';
	import TrackList from '$lib/components/detail/TrackList.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import OfflineBadge from '$lib/components/OfflineBadge.svelte';
	import ProgressLine from '$lib/components/ProgressLine.svelte';
	import ItemCard from '$lib/components/ItemCard.svelte';
	import AddToCollectionDialog from '$lib/components/collections/AddToCollectionDialog.svelte';
	import EditItemDialog from '$lib/components/items/EditItemDialog.svelte';
	import IdentifyDialog from '$lib/components/items/IdentifyDialog.svelte';
	import Menu, { type MenuItem } from '$lib/components/Menu.svelte';
	import Row from '$lib/components/Row.svelte';
	import SectionHeader from '$lib/components/SectionHeader.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import { formatClock, formatDate, formatRuntime, metaLine, plural } from '$lib/format';
	import { episodeCode, isOffline, KIND_ICON, KIND_LABEL, OFFLINE_HINT } from '$lib/media';
	import { isFavorite, isPlayed, progressOf, resumeMs } from '$lib/player/resume';
	import { HAS_PLAY, nextInSeason, playerHref, resolvePlayTarget, seriesNextUp } from '$lib/player/target';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	const id = $derived(Number(page.params.id));

	let item = $state<ItemDetail | null>(null);
	let error = $state<unknown>(null);
	let children = $state<Item[]>([]);
	let childrenLoading = $state(false);
	let similar = $state<Item[]>([]);
	/** Root series, for the backdrop of seasons and episodes. */
	let series = $state<ItemDetail | null>(null);
	let editOpen = $state(false);
	let identifyOpen = $state(false);
	let collectOpen = $state(false);
	/** Bumped to refetch the episodes on screen (after a watched toggle on the series). */
	let episodesVersion = $state(0);

	const byIndex = (a: Item, b: Item) =>
		(a.parentIndexNumber ?? 0) - (b.parentIndexNumber ?? 0) || (a.indexNumber ?? 1e9) - (b.indexNumber ?? 1e9) || a.title.localeCompare(b.title, 'fr');

	async function load(silent = false, signal?: AbortSignal) {
		const want = id;
		if (!silent) {
			item = null;
			error = null;
			children = [];
			similar = [];
			series = null;
		}
		try {
			const d = await api.item(want, signal);
			if (want !== id) return;
			if (d.kind === 'track' && d.parentId) {
				goto(`/item/${d.parentId}`, { replaceState: true });
				return;
			}
			item = d;
			loadRelated(d, silent, signal);
		} catch (err) {
			if (!isAbort(err) && !silent) error = err;
		}
	}

	function loadRelated(d: ItemDetail, silent: boolean, signal?: AbortSignal) {
		if (['series', 'season', 'artist', 'album'].includes(d.kind)) {
			if (!silent) childrenLoading = true;
			api
				.items({ parent: d.id, limit: 200, sort: d.kind === 'artist' ? 'year' : 'title', order: d.kind === 'artist' ? 'desc' : undefined }, signal)
				.then((p) => (children = d.kind === 'artist' ? p.items : [...p.items].sort(byIndex)))
				.catch(() => {})
				.finally(() => (childrenLoading = false));
		}
		if (['movie', 'series', 'artist', 'album'].includes(d.kind)) {
			api
				.similar(d.id, 16, signal)
				.then((s) => (similar = s))
				.catch(() => (similar = []));
		}
		const root = d.ancestors[0];
		if ((d.kind === 'season' || d.kind === 'episode') && root && series?.id !== root.id) {
			api
				.item(root.id, signal)
				.then((s) => (series = s))
				.catch(() => {});
		}
	}

	$effect(() => {
		void id;
		const ctrl = new AbortController();
		load(false, ctrl.signal);
		return () => ctrl.abort();
	});

	// Metadata refreshed or new artwork for this item: reload in place.
	$effect(() => {
		const want = id;
		return events.debounced(['item.updated', 'item.image'], () => load(true), 700, (ev) => (ev.data as { id?: number } | undefined)?.id === want);
	});

	// ——— Series: seasons as tabs ———
	const seasonParam = $derived(page.url.searchParams.get('saison'));
	const seasons = $derived(item?.kind === 'series' ? children : []);
	const selectedSeason = $derived(seasons.find((s) => String(s.id) === seasonParam) ?? seasons[0]);
	let episodes = $state<Item[]>([]);
	let episodesLoading = $state(false);

	$effect(() => {
		const s = selectedSeason;
		const silent = untrack(() => episodesVersion > 0 && episodes.length > 0 && episodes[0]?.parentId === s?.id);
		void episodesVersion;
		if (!silent) episodes = [];
		if (!s) return;
		if (!silent) episodesLoading = true;
		const ctrl = new AbortController();
		api
			.items({ parent: s.id, limit: 200 }, ctrl.signal)
			.then((p) => (episodes = [...p.items].sort(byIndex)))
			.catch(() => {})
			.finally(() => (episodesLoading = false));
		return () => ctrl.abort();
	});

	function selectSeason(s: Item, focus = false) {
		const url = new URL(page.url);
		url.searchParams.set('saison', String(s.id));
		goto(url, { replaceState: true, keepFocus: true, noScroll: true });
		if (focus) requestAnimationFrame(() => document.getElementById(`tab-${s.id}`)?.focus());
	}

	function onTabsKey(e: KeyboardEvent) {
		if (!selectedSeason) return;
		const i = seasons.indexOf(selectedSeason);
		let next = -1;
		if (e.key === 'ArrowRight') next = (i + 1) % seasons.length;
		else if (e.key === 'ArrowLeft') next = (i - 1 + seasons.length) % seasons.length;
		else if (e.key === 'Home') next = 0;
		else if (e.key === 'End') next = seasons.length - 1;
		if (next < 0) return;
		e.preventDefault();
		selectSeason(seasons[next], true);
	}

	// ——— Derived presentation ———
	const people = $derived(item?.people ?? []);
	const cast = $derived(people.filter((p) => p.kind === 'actor' || p.kind === 'guest'));
	const directors = $derived([...new Map(people.filter((p) => p.kind === 'director').map((p) => [p.id, p])).values()]);
	const writers = $derived([...new Map(people.filter((p) => p.kind === 'writer').map((p) => [p.id, p])).values()]);
	const studios = $derived(item?.studios ?? []);

	const backdrop = $derived(
		item ? (item.kind === 'episode' ? firstImage(item, 'thumb', 'backdrop') : firstImage(item, 'backdrop')) ?? firstImage(series, 'backdrop') : undefined
	);
	const poster = $derived(item ? firstImage(item, 'poster') ?? (item.kind === 'season' ? firstImage(series, 'poster') : undefined) : undefined);
	const artwork = $derived(
		!item ? 'poster' : item.kind === 'episode' ? 'none' : item.kind === 'artist' ? 'round' : item.kind === 'album' ? 'square' : 'poster'
	);
	const library = $derived(item ? libraries.get(item.libraryId) : undefined);

	const crumbs = $derived.by(() => {
		if (!item) return [];
		const list: { label: string; href?: string }[] = [];
		if (library) list.push({ label: library.name, href: `/bibliotheque/${library.id}` });
		for (const a of item.ancestors) list.push({ label: a.title, href: `/item/${a.id}` });
		if (list.length) list.push({ label: item.title });
		return list;
	});

	const eyebrow = $derived.by(() => {
		if (!item) return undefined;
		if (item.kind === 'episode') return episodeCode(item);
		if (item.kind === 'season') return item.ancestors[0]?.title;
		if (item.kind === 'album') return 'Album';
		const label = KIND_LABEL[item.kind].one;
		return label.charAt(0).toUpperCase() + label.slice(1);
	});

	const albumArtist = $derived(item?.kind === 'album' ? item.ancestors.find((a) => a.kind === 'artist') : undefined);
	const albumDuration = $derived(item?.kind === 'album' ? children.reduce((sum, t) => sum + (t.runtimeMs ?? 0), 0) : 0);

	const collectable = $derived(!!item && ['movie', 'series', 'artist', 'album'].includes(item.kind));
	const menuItems = $derived<MenuItem[]>(
		item
			? [
					...(collectable ? [{ label: 'Ajouter à une collection…', icon: 'list-plus' as const, onselect: () => (collectOpen = true) }] : []),
					...(session.isAdmin
						? [
								{ label: 'Actualiser les métadonnées', icon: 'refresh' as const, onselect: refresh, separated: collectable },
								...(['movie', 'series', 'artist', 'album'].includes(item.kind) ? [{ label: 'Identifier…', icon: 'wand' as const, onselect: () => (identifyOpen = true) }] : []),
								{ label: 'Modifier…', icon: 'edit' as const, onselect: () => (editOpen = true) }
							]
						: [])
				]
			: []
	);

	async function refresh() {
		if (!item) return;
		try {
			await api.refreshItem(item.id);
			toasts.show('Actualisation demandée', { detail: `Les métadonnées de « ${item.title} » vont être mises à jour.` });
		} catch (err) {
			toasts.show('Actualisation impossible', { detail: errorMessage(err) });
		}
	}

	/** Children the "Lecture" button can pick from without another request. */
	const known = $derived(item?.kind === 'season' || item?.kind === 'album' ? children : []);
	let resolving = $state(false);

	// ——— Progress (M6) ———
	/** Next-up episode of a series (first unwatched or in progress), resolved in the background. */
	let nextUp = $state<Item | null>(null);
	$effect(() => {
		const it = item;
		const list = seasons;
		void episodesVersion;
		if (it?.kind !== 'series' || !list.length) {
			nextUp = null;
			return;
		}
		const ctrl = new AbortController();
		seriesNextUp(it.id, ctrl.signal, list)
			.then((n) => (nextUp = n))
			.catch(() => {});
		return () => ctrl.abort();
	});

	/** Episode the Lecture button plays for a series / season. */
	const upNext = $derived(item?.kind === 'series' ? nextUp : item?.kind === 'season' ? (nextInSeason(children) ?? null) : null);
	/** Movie / episode resume point, or the next-up episode's. */
	const resumeAt = $derived(item?.kind === 'movie' || item?.kind === 'episode' ? resumeMs(item) : resumeMs(upNext));
	const upNextCode = $derived(upNext ? episodeCode(upNext) : undefined);
	/** Started: something watched already (else a plain "Lecture"). */
	const started = $derived(!!upNext && (resumeAt > 0 || (item?.unplayedCount !== undefined && upNext.indexNumber !== undefined && !(upNext.indexNumber <= 1 && (upNext.parentIndexNumber ?? 1) <= 1))));
	const playLabel = $derived.by(() => {
		if (resumeAt) return upNextCode && item?.kind !== 'episode' ? `Reprendre ${upNextCode}` : `Reprendre à ${formatClock(resumeAt / 1000)}`;
		if (upNextCode && started && (item?.kind === 'series' || item?.kind === 'season')) return `Lecture ${upNextCode}`;
		return 'Lecture';
	});
	const played = $derived(isPlayed(item));
	const favorite = $derived(isFavorite(item));
	const canWatch = $derived(!!item && ['movie', 'episode', 'series', 'season'].includes(item.kind));
	const canFavorite = $derived(!!item && ['movie', 'episode', 'series'].includes(item.kind));
	const progress = $derived(item?.kind === 'movie' || item?.kind === 'episode' ? progressOf(item) : undefined);
	let savingWatch = $state(false);

	async function togglePlayed() {
		if (!item || savingWatch) return;
		const it = item;
		const want = !played;
		const before = { userData: it.userData, unplayedCount: it.unplayedCount };
		savingWatch = true;
		// Optimistic: the button flips at once.
		if (it.kind === 'series' || it.kind === 'season') it.unplayedCount = want ? 0 : Math.max(1, it.unplayedCount ?? 1);
		else it.userData = { ...normalizeUserData(it.userData), played: want, positionMs: want ? 0 : (it.userData?.positionMs ?? 0) };
		try {
			const d = await api.putUserData(it.id, { played: want });
			if (it.kind === 'movie' || it.kind === 'episode') it.userData = d;
			toasts.show(want ? `« ${it.title} » marqué comme vu` : `« ${it.title} » marqué comme non vu`);
			if (it.kind === 'series' || it.kind === 'season') {
				// Every episode changed: refresh the counts and the lists on screen.
				await load(true);
				episodesVersion++;
			}
		} catch (err) {
			it.userData = before.userData;
			it.unplayedCount = before.unplayedCount;
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		} finally {
			savingWatch = false;
		}
	}

	async function toggleFavorite() {
		if (!item) return;
		const it = item;
		const want = !favorite;
		const before = it.userData;
		it.userData = { ...normalizeUserData(it.userData), favorite: want };
		try {
			it.userData = await api.putUserData(it.id, { favorite: want });
			toasts.show(want ? 'Ajouté aux favoris' : 'Retiré des favoris', { detail: it.title, duration: 3000 });
		} catch (err) {
			it.userData = before;
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}

	/** Watched toggle on one episode of the list: patch it in place, then the counters. */
	async function toggleEpisode(ep: Item) {
		const want = !ep.userData?.played;
		const isSeries = item?.kind === 'series';
		const before = ep.userData;
		const patch = (u: Item['userData']) => {
			const list = isSeries ? episodes : children;
			const idx = list.findIndex((e) => e.id === ep.id);
			if (idx >= 0) list[idx] = { ...list[idx], userData: u };
		};
		patch({ ...normalizeUserData(before), played: want, positionMs: want ? 0 : (before?.positionMs ?? 0) });
		try {
			patch(await api.putUserData(ep.id, { played: want }));
			const delta = want ? -1 : 1;
			if (item && item.unplayedCount !== undefined) item.unplayedCount = Math.max(0, item.unplayedCount + delta);
			const season = isSeries ? seasons.find((s) => s.id === ep.parentId) : undefined;
			if (season && season.unplayedCount !== undefined) season.unplayedCount = Math.max(0, season.unplayedCount + delta);
			if (isSeries) nextUp = null;
			if (isSeries) seriesNextUp(item!.id, undefined, seasons).then((n) => (nextUp = n), () => {});
		} catch (err) {
			patch(before);
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}

	async function play(fromStart = false) {
		if (!item || resolving || offline) return;
		resolving = true;
		try {
			if ((item.kind === 'movie' || item.kind === 'episode') && !fromStart) {
				// Explicit start: the player skips its own resume prompt.
				goto(playerHref(item.id, { t: resumeAt / 1000 }));
				return;
			}
			if (fromStart) {
				const target = item.kind === 'movie' || item.kind === 'episode' ? item.id : upNext?.id;
				if (target) {
					goto(playerHref(target, { t: 0 }));
					return;
				}
			}
			if (upNext) {
				goto(playerHref(upNext.id, { t: resumeMs(upNext) / 1000 }));
				return;
			}
			const target = await resolvePlayTarget(item, known);
			if (target) goto(playerHref(target));
			else toasts.show('Rien à lire pour l’instant', { detail: 'Aucun épisode ou piste n’a encore été trouvé.' });
		} catch (err) {
			toasts.show('Lecture impossible', { detail: errorMessage(err) });
		} finally {
			resolving = false;
		}
	}

	const playable = $derived(item ? HAS_PLAY.includes(item.kind) : false);
	const offline = $derived(isOffline(item));
	const childLabel = $derived.by(() => {
		if (!item?.childCount) return undefined;
		const map = { series: ['saison', 'saisons'], season: ['épisode', 'épisodes'], artist: ['album', 'albums'], album: ['piste', 'pistes'] } as const;
		const m = map[item.kind as keyof typeof map];
		return m ? plural(item.childCount, m[0], m[1]) : undefined;
	});
</script>

<svelte:head>
	<title>{item?.title ?? 'Chargement…'} — Aether</title>
</svelte:head>

{#if error}
	<div class="page">
		<EmptyState icon="alert" title={(error as { status?: number }).status === 404 ? 'Titre introuvable' : 'Impossible d’afficher ce titre'} role="alert">
			<p>{errorMessage(error)}</p>
			{#snippet actions()}
				<Button href="/" icon="chevron-left">Retour à l’accueil</Button>
				<Button icon="refresh" onclick={() => load()}>Réessayer</Button>
			{/snippet}
		</EmptyState>
	</div>
{:else if !item}
	<div class="loading" aria-busy="true">
		<span class="visually-hidden">Chargement…</span>
		<div class="sk-head">
			<Skeleton ratio="2 / 3" radius="lg" class="sk-poster" />
			<div class="sk-lines">
				<Skeleton width="50%" height="44px" radius="sm" />
				<Skeleton width="30%" height="14px" radius="sm" />
				<Skeleton lines={3} width="80%" />
			</div>
		</div>
	</div>
{:else}
	{@const it = item}
	<article class="detail" class:offline>
		<DetailHeader
			title={it.title}
			{eyebrow}
			{backdrop}
			{poster}
			logo={firstImage(it, 'logo')}
			{artwork}
			icon={KIND_ICON[it.kind]}
			year={it.year}
			crumbs={it.kind === 'season' || it.kind === 'episode' || it.kind === 'album' ? crumbs : undefined}
			tagline={it.tagline}
			overview={it.overview}
		>
			{#snippet meta()}
				{#if it.kind === 'album' && albumArtist}
					<a class="artist-link dot" href="/item/{albumArtist.id}">{albumArtist.title}</a>
				{/if}
				{#if it.kind === 'episode'}
					{#if it.airDate || it.premiereDate}<span class="dot">{formatDate(it.airDate ?? it.premiereDate)}</span>{/if}
				{:else if it.year}
					<span class="dot tabular">{it.year}{it.kind === 'series' && it.endDate ? `–${it.endDate.slice(0, 4)}` : ''}</span>
				{/if}
				{#if it.kind === 'album'}
					{#if childLabel}<span class="dot">{childLabel}</span>{/if}
					{#if albumDuration}<span class="dot tabular">{formatRuntime(albumDuration)}</span>{/if}
				{:else if it.kind !== 'movie' && it.kind !== 'episode' && childLabel}
					<span class="dot">{childLabel}</span>
				{/if}
				{#if formatRuntime(it.runtimeMs) && (it.kind === 'movie' || it.kind === 'episode')}<span class="dot tabular">{formatRuntime(it.runtimeMs)}</span>{/if}
				{#if it.officialRating}<span class="rating">{it.officialRating}</span>{/if}
				{#if it.communityRating}
					<span class="score tabular"><Icon name="star" size={14} filled />{(Math.round(it.communityRating * 10) / 10).toLocaleString('fr-FR')}</span>
				{/if}
				{#if it.status}<Tag variant="image">{it.status === 'continuing' ? 'En cours' : 'Terminée'}</Tag>{/if}
				{#if it.video}
					<Tag variant="image">{it.video.resolution}</Tag>
					{#if it.video.hdr}<Tag variant="image">{it.video.hdr}</Tag>{/if}
				{/if}
				{#if it.identifyStatus === 'unmatched'}<Tag variant="image" icon="wand">À identifier</Tag>{/if}
				{#if offline}<OfflineBadge />{/if}
			{/snippet}

			{#snippet chips()}
				{#each it.genres as g (g)}
					<a class="genre" href="/parcourir?genre={encodeURIComponent(g)}">{g}</a>
				{/each}
			{/snippet}

			{#snippet actions()}
				{#if playable}
					<Button
						variant="primary"
						size="lg"
						icon="play"
						iconFilled
						aria-busy={resolving}
						aria-disabled={offline ? 'true' : undefined}
						title={offline ? OFFLINE_HINT : undefined}
						aria-describedby={offline ? 'offline-note' : undefined}
						onclick={() => play()}>{playLabel}</Button
					>
					{#if resumeAt && !offline}
						<Button size="lg" icon="refresh" onclick={() => play(true)}>Depuis le début</Button>
					{/if}
					{#if offline}<span id="offline-note" class="visually-hidden">{OFFLINE_HINT} : les fichiers de ce titre sont sur une source hors ligne.</span>{/if}
				{/if}
				{#if canWatch}
					<Button
						size="lg"
						icon={played ? 'check-circle' : 'check'}
						class={played ? 'is-on' : undefined}
						aria-pressed={played}
						aria-busy={savingWatch}
						title={played ? 'Marquer comme non vu' : undefined}
						onclick={togglePlayed}>{played ? 'Vu' : 'Marquer comme vu'}</Button
					>
				{/if}
				{#if canFavorite}
					<Button
						size="lg"
						round
						icon="heart"
						iconFilled={favorite}
						class={favorite ? 'is-on' : undefined}
						aria-pressed={favorite}
						aria-label={favorite ? 'Retirer des favoris' : 'Ajouter aux favoris'}
						title={favorite ? 'Retirer des favoris' : 'Ajouter aux favoris'}
						onclick={toggleFavorite}
					/>
				{/if}
				{#if menuItems.length}<Menu label="Plus d’actions" items={menuItems} icon="more" text="Plus" />{/if}
			{/snippet}

			{#snippet extra()}
				{#if progress && resumeAt}
					<div class="resume-line">
						<ProgressLine value={progress} variant="inline" />
						<span class="tabular">{formatClock(resumeAt / 1000)} sur {formatClock((it.runtimeMs ?? 0) / 1000)} · reste {formatRuntime((it.runtimeMs ?? 0) - resumeAt)}</span>
					</div>
				{/if}
			{/snippet}
		</DetailHeader>

		<div class="sections">
			{#if it.kind === 'series'}
				<section class="block" aria-labelledby="seasons-title">
					<SectionHeader title="Saisons" id="seasons-title" />
					{#if childrenLoading}
						<Skeleton width="320px" height="38px" radius="full" />
					{:else if seasons.length}
						<!-- svelte-ignore a11y_interactive_supports_focus -->
						<div class="tabs" role="tablist" aria-label="Saisons" onkeydown={onTabsKey}>
							{#each seasons as s (s.id)}
								{@const selected = s.id === selectedSeason?.id}
								<button
									type="button"
									role="tab"
									id="tab-{s.id}"
									aria-selected={selected}
									aria-controls="season-panel"
									tabindex={selected ? 0 : -1}
									onclick={() => selectSeason(s)}
								>
									{s.title}
									{#if s.unplayedCount === 0 && s.childCount}
										<span class="tab-done" title="Saison vue"><Icon name="check" size={14} /><span class="visually-hidden">(vue)</span></span>
									{:else if s.unplayedCount}
										<span class="tab-count unseen tabular" title="{s.unplayedCount} non vu{s.unplayedCount > 1 ? 's' : ''}">{s.unplayedCount}<span class="visually-hidden"> non vus</span></span>
									{:else if s.childCount}<span class="tab-count tabular">{s.childCount}</span>{/if}
								</button>
							{/each}
						</div>
						<div class="season-panel" id="season-panel" role="tabpanel" aria-labelledby="tab-{selectedSeason?.id}">
							{#if selectedSeason}
								<div class="season-head">
									<p>{metaLine(selectedSeason.year, selectedSeason.childCount ? plural(selectedSeason.childCount, 'épisode', 'épisodes') : undefined)}</p>
									<a href="/item/{selectedSeason.id}">Voir la saison</a>
								</div>
							{/if}
							<EpisodeList episodes={episodes} loading={episodesLoading} label="Épisodes de {selectedSeason?.title ?? 'la saison'}" ontoggleplayed={toggleEpisode} />
						</div>
					{:else}
						<p class="muted">Aucune saison pour l’instant.</p>
					{/if}
				</section>
			{/if}

			{#if it.kind === 'season'}
				<section class="block" aria-labelledby="eps-title">
					<SectionHeader title="Épisodes" id="eps-title" />
					<EpisodeList episodes={children} loading={childrenLoading} label="Épisodes" ontoggleplayed={toggleEpisode} />
				</section>
			{/if}

			{#if it.kind === 'album'}
				<section class="block narrow" aria-labelledby="tracks-title">
					<SectionHeader title="Pistes" id="tracks-title" />
					<TrackList tracks={children} loading={childrenLoading} />
				</section>
			{/if}

			{#if it.kind === 'artist'}
				<section class="block" aria-labelledby="albums-title">
					<SectionHeader title="Albums" id="albums-title" />
					{#if childrenLoading}
						<div class="grid">{#each [0, 1, 2, 3] as i (i)}<Skeleton ratio="1 / 1" radius="lg" />{/each}</div>
					{:else if children.length}
						<ul class="grid" role="list">
							{#each children as a (a.id)}<li><ItemCard item={a} subtitle={a.year ? String(a.year) : undefined} /></li>{/each}
						</ul>
					{:else}
						<p class="muted">Aucun album.</p>
					{/if}
				</section>
			{/if}

			{#if cast.length}<CastRow people={cast} />{/if}

			{#if directors.length || writers.length || studios.length || it.originalTitle || it.premiereDate || it.collection}
				<section class="block" aria-labelledby="info-title">
					<SectionHeader title="Informations" id="info-title" />
					<dl class="facts panel">
						{#if directors.length}
							<div><dt>Réalisation</dt><dd>{#each directors as p, i (p.id)}{i ? ', ' : ''}<a href="/parcourir?personne={p.id}&nom={encodeURIComponent(p.name)}">{p.name}</a>{/each}</dd></div>
						{/if}
						{#if writers.length}
							<div><dt>Scénario</dt><dd>{#each writers as p, i (p.id)}{i ? ', ' : ''}<a href="/parcourir?personne={p.id}&nom={encodeURIComponent(p.name)}">{p.name}</a>{/each}</dd></div>
						{/if}
						{#if studios.length}<div><dt>{it.kind === 'album' ? 'Label' : 'Studio'}</dt><dd>{studios.join(', ')}</dd></div>{/if}
						{#if it.originalTitle && it.originalTitle !== it.title}<div><dt>Titre original</dt><dd>{it.originalTitle}</dd></div>{/if}
						{#if it.premiereDate && it.kind !== 'episode'}<div><dt>{it.kind === 'series' ? 'Première diffusion' : 'Sortie'}</dt><dd>{formatDate(it.premiereDate)}</dd></div>{/if}
						{#if it.collection}<div><dt>Collection</dt><dd>{it.collection}</dd></div>{/if}
						<div><dt>Ajouté le</dt><dd>{formatDate(it.dateAdded)}</dd></div>
					</dl>
				</section>
			{/if}

			{#if it.kind === 'movie' || it.kind === 'episode'}
				<TechPanel files={it.mediaFiles} showPath={session.isAdmin} />
			{/if}

			{#if similar.length}
				<Row title={it.kind === 'artist' ? 'Artistes similaires' : 'Similaires'} items={similar} key={(s) => s.id}>
					{#snippet item(s)}<ItemCard item={s} />{/snippet}
				</Row>
			{/if}
		</div>
	</article>

	<EditItemDialog bind:open={editOpen} item={it} onsaved={(u) => (item = { ...u, ancestors: u.ancestors.length ? u.ancestors : it.ancestors })} />
	<IdentifyDialog bind:open={identifyOpen} item={it} onidentified={() => setTimeout(() => load(true), 1500)} />
	{#if collectable}<AddToCollectionDialog bind:open={collectOpen} item={it} />{/if}
{/if}

<style>
	/* Files on an offline source: artwork greyed (the badge and disabled "Lecture" say why). */
	.detail.offline :global(.detail-header .art),
	.detail.offline :global(.detail-header .backdrop) {
		opacity: 0.45;
		filter: grayscale(0.7);
	}

	.detail {
		padding-bottom: var(--sp-9);
	}

	.sections {
		display: grid;
		gap: var(--sp-7);
		padding: var(--sp-6) var(--gutter) 0;
		max-width: 1560px;
	}

	.season-panel {
		max-width: 1100px;
	}

	.block {
		min-width: 0;
	}

	.narrow {
		max-width: 880px;
	}

	.muted {
		color: var(--text-muted);
	}

	.artist-link {
		color: var(--text);
		font-weight: var(--fw-bold);
		text-decoration: underline;
		text-decoration-color: color-mix(in srgb, currentColor 40%, transparent);
		text-underline-offset: 3px;
		border-radius: var(--r-xs);
	}

	.rating {
		padding: 1px 7px;
		border-radius: var(--r-xs);
		box-shadow: inset 0 0 0 1px currentColor;
		font-size: var(--fs-micro);
	}

	.score {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}

	.score :global(svg) {
		color: var(--accent);
	}

	.genre {
		display: inline-flex;
		align-items: center;
		height: 30px;
		padding: 0 13px;
		border-radius: var(--r-full);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
		color: var(--text);
		background: color-mix(in srgb, var(--bg) 45%, transparent);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.genre:hover {
		background: color-mix(in srgb, var(--bg) 70%, transparent);
	}

	.tabs {
		display: flex;
		gap: var(--sp-2);
		margin: 0 calc(-1 * var(--gutter)) var(--sp-4);
		padding: 2px var(--gutter);
		overflow-x: auto;
		scrollbar-width: none;
	}

	[role='tab'] {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		flex: none;
		height: 38px;
		padding: 0 var(--sp-4);
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-weight: var(--fw-bold);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	[role='tab']:hover {
		color: var(--text);
	}

	[role='tab'][aria-selected='true'] {
		background: var(--surface-active);
		color: var(--text);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 30%, transparent);
	}

	.tab-done {
		display: inline-grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.tab-count.unseen {
		background: var(--accent-soft);
		color: var(--accent-ink);
		font-weight: var(--fw-heavy);
	}

	.resume-line {
		display: grid;
		gap: var(--sp-2);
		max-width: 420px;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.detail :global(.btn.secondary.is-on) {
		color: var(--accent-ink);
	}

	.detail :global(.btn.secondary.is-on svg) {
		color: var(--accent);
	}

	.tab-count {
		min-width: 20px;
		padding: 0 6px;
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 8%, transparent);
		font-size: var(--fs-micro);
		line-height: 20px;
	}

	.season-head {
		display: flex;
		justify-content: space-between;
		gap: var(--sp-3);
		margin-bottom: var(--sp-2);
		padding: 0 var(--sp-2);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.season-head a {
		color: var(--accent-ink);
		font-weight: var(--fw-bold);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: var(--sp-5) var(--gap);
		max-width: 1200px;
	}

	.facts {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 240px), 1fr));
		gap: var(--sp-4) var(--sp-6);
		padding: var(--sp-5);
	}

	.facts div {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	dt {
		font-size: var(--fs-micro);
		font-weight: var(--fw-medium);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		margin: 0;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	dd a:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.loading {
		padding: clamp(var(--sp-8), 14vw, 220px) var(--gutter) var(--sp-6);
	}

	.sk-head {
		display: flex;
		align-items: flex-end;
		gap: var(--sp-6);
	}

	.sk-head :global(.sk-poster) {
		width: clamp(170px, 15vw, 240px) !important;
		flex: none;
	}

	.sk-lines {
		display: grid;
		gap: var(--sp-3);
		flex: 1;
	}

	@media (max-width: 767px) {
		.sections {
			gap: var(--sp-6);
		}

		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.sk-head {
			flex-direction: column;
			align-items: flex-start;
		}

		.sk-head :global(.sk-poster) {
			width: 132px !important;
		}
	}
</style>
