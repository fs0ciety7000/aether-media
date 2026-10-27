<script lang="ts">
	import Hero from '$lib/components/Hero.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import Row from '$lib/components/Row.svelte';
	import ThumbCard from '$lib/components/ThumbCard.svelte';
	import BackdropArt from '$lib/demo/BackdropArt.svelte';
	import PosterArt from '$lib/demo/PosterArt.svelte';
	import { collections, continueWatching, dominant, filmById, films, isEpisode } from '$lib/demo/data';
	import { formatDuration, metaLine } from '$lib/format';
	import { toasts } from '$lib/toast.svelte';

	const featured = films.slice(0, 4).map((f) => ({
		...f,
		meta: metaLine('Film', f.year, formatDuration(f.minutes)),
		color: dominant(f.hue, f.sat)
	}));

	const recent = [...films].sort((a, b) => b.year - a.year);

	function remaining(minutes: number, progress: number) {
		return `Reste ${formatDuration(Math.max(1, Math.round(minutes * (1 - progress))))}`;
	}
</script>

<svelte:head>
	<title>Accueil — Aether</title>
</svelte:head>

<Hero
	items={featured}
	onplay={(f) => toasts.show(`Lecture de « ${f.title} »`, { detail: 'Le lecteur arrive dans une prochaine version.' })}
	onsecondary={(f) => toasts.show(f.title, { detail: `Réalisé par ${f.director}` })}
>
	{#snippet backdrop(f)}
		<BackdropArt hue={f.hue} sat={f.sat} scene={f.scene} />
	{/snippet}
</Hero>

<div class="rows">
	<Row title="Reprendre" items={continueWatching} key={(it) => it.id} itemWidth="clamp(248px, 24vw, 336px)">
		{#snippet item(it)}
			{#if isEpisode(it)}
				<ThumbCard
					href="/series"
					eyebrow={metaLine(it.show, `S${it.season}`, `É${it.episode}`)}
					title={it.title}
					detail={remaining(it.minutes, it.progress)}
					progress={it.progress}
					color={dominant(it.hue)}
				>
					{#snippet art()}<BackdropArt hue={it.hue} scene={it.scene} />{/snippet}
				</ThumbCard>
			{:else}
				<ThumbCard
					href="/films"
					eyebrow={metaLine('Film', it.year)}
					title={it.title}
					detail={remaining(it.minutes, it.progress)}
					progress={it.progress}
					color={dominant(it.hue, it.sat)}
				>
					{#snippet art()}<BackdropArt hue={it.hue} sat={it.sat} scene={it.scene} />{/snippet}
				</ThumbCard>
			{/if}
		{/snippet}
	</Row>

	<Row title="Récemment ajoutés" href="/films" items={recent} key={(f) => f.id}>
		{#snippet item(f)}
			<PosterCard href="/films" title={f.title} year={f.year} progress={f.progress} color={dominant(f.hue, f.sat)}>
				{#snippet art()}
					<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} sat={f.sat} layout={f.layout} />
				{/snippet}
			</PosterCard>
		{/snippet}
	</Row>

	<Row title="Collections" href="/collections" items={collections} key={(c) => c.id} itemWidth="clamp(248px, 24vw, 336px)">
		{#snippet item(c)}
			<ThumbCard href="/collections" eyebrow="{c.count} films" title={c.title} playable={false} color="var(--surface)">
				{#snippet art()}
					<div class="stack">
						{#each c.filmIds.map(filmById) as f, i (f.id)}
							<div class="stack-item" class:center={i === 1} style:--i={i}>
								<PosterArt title={f.title} credit={f.director} hue={f.hue} sat={f.sat} layout={f.layout} />
							</div>
						{/each}
					</div>
				{/snippet}
			</ThumbCard>
		{/snippet}
	</Row>
</div>

<style>
	.rows {
		display: grid;
		gap: var(--sp-7);
		padding: var(--sp-6) 0 var(--sp-9);
	}

	@media (min-width: 1200px) {
		.rows {
			gap: var(--sp-8);
		}
	}

	/* Collection tile: three posters fanned on a quiet surface. */
	.stack {
		position: absolute;
		inset: 0;
		background: linear-gradient(160deg, var(--surface-raised), var(--surface));
	}

	.stack-item {
		position: absolute;
		top: 20%;
		height: 92%;
		aspect-ratio: 2 / 3;
		left: calc(31% + var(--i) * 19%);
		translate: -50% 0;
		border-radius: 4px;
		overflow: hidden;
		box-shadow: var(--shadow-2);
		z-index: 1;
		filter: brightness(0.82);
	}

	.stack-item.center {
		top: 12%;
		z-index: 2;
		filter: none;
	}
</style>
