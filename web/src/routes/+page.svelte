<script lang="ts">
	import CategoriesPanel from '$lib/components/CategoriesPanel.svelte';
	import CategoryTile from '$lib/components/CategoryTile.svelte';
	import ContinueBento from '$lib/components/ContinueBento.svelte';
	import ContinueTile from '$lib/components/ContinueTile.svelte';
	import Hero from '$lib/components/Hero.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import Row from '$lib/components/Row.svelte';
	import ThumbCard from '$lib/components/ThumbCard.svelte';
	import BackdropArt from '$lib/demo/BackdropArt.svelte';
	import PosterArt from '$lib/demo/PosterArt.svelte';
	import { continueWatching, dominant, films, genres, isEpisode } from '$lib/demo/data';
	import { formatClock, formatDuration, metaLine } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';

	const badges = ['Tendance', 'Nouveauté', 'Coup de cœur', 'Tendance'];
	const featured = films.slice(0, 4).map((f, i) => ({
		...f,
		tags: f.genres,
		badge: badges[i],
		meta: metaLine(f.year, formatDuration(f.minutes), f.director),
		color: dominant(f.hue, f.hue2)
	}));

	const youMightLike = films.slice(4).concat(films.slice(0, 2));
	const recent = [...films].sort((a, b) => b.year - a.year).slice(0, 8);
</script>

<svelte:head>
	<title>Accueil — Aether</title>
</svelte:head>

<h1 class="visually-hidden">Accueil</h1>

<div class="home">
	<div class="top">
		<Hero
			class="hero-card"
			items={featured}
			onplay={(f) => toasts.show(`Lecture de « ${f.title} »`, { detail: 'Le lecteur arrive dans une prochaine version.' })}
			onsecondary={(f) => toasts.show(f.title, { detail: `Réalisé par ${f.director}` })}
		>
			{#snippet backdrop(f)}
				<BackdropArt hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} focus={0.64} />
			{/snippet}
		</Hero>

		<CategoriesPanel items={genres} href="/genres" key={(g) => g.id}>
			{#snippet tile(g)}
				<CategoryTile label={g.label} href="/genres/{g.id}" color={dominant(g.hue, g.hue2)}>
					{#snippet art()}<BackdropArt hue={g.hue} hue2={g.hue2} scene={g.scene} focus={0.5} />{/snippet}
				</CategoryTile>
			{/snippet}
		</CategoriesPanel>
	</div>

	<ContinueBento items={continueWatching} href="/reprendre" key={(it) => it.id}>
		{#snippet tile(it, { featured })}
			{@const total = it.minutes * 60}
			<ContinueTile
				href={isEpisode(it) ? '/series' : '/films'}
				title={isEpisode(it) ? it.show : it.title}
				subtitle={isEpisode(it) ? metaLine(`S${it.season} · É${it.episode}`, it.title) : metaLine('Film', it.year)}
				elapsed={formatClock(total * it.progress)}
				total={formatClock(total)}
				progress={it.progress}
				color={dominant(it.hue, it.hue2)}
				{featured}
			>
				{#snippet art()}<BackdropArt hue={it.hue} hue2={it.hue2} scene={it.scene} focus={featured ? 0.55 : 0.5} />{/snippet}
			</ContinueTile>
		{/snippet}
	</ContinueBento>

	<Row title="Vous aimerez aussi" href="/explorer" items={youMightLike} key={(f) => f.id}>
		{#snippet item(f)}
			<PosterCard href="/films" title={f.title} year={f.year} color={dominant(f.hue, f.hue2)}>
				{#snippet art()}
					<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />
				{/snippet}
			</PosterCard>
		{/snippet}
	</Row>

	<Row title="Récemment ajoutés" href="/recents" items={recent} key={(f) => f.id} itemWidth="clamp(232px, 21vw, 300px)">
		{#snippet item(f)}
			<ThumbCard
				href="/films"
				title={f.title}
				eyebrow={metaLine(f.genres[0], f.year)}
				detail={formatDuration(f.minutes)}
				progress={f.progress}
				color={dominant(f.hue, f.hue2)}
			>
				{#snippet art()}<BackdropArt hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} focus={0.6} />{/snippet}
			</ThumbCard>
		{/snippet}
	</Row>
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

	@media (max-width: 1023px) {
		.top {
			grid-template-columns: minmax(0, 1fr);
		}

		.top :global(.categories .grid) {
			grid-template-columns: repeat(4, minmax(0, 1fr));
			grid-auto-rows: 72px;
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
	}
</style>
