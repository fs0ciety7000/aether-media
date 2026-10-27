<script lang="ts">
	import { tick } from 'svelte';
	import Button from '$lib/components/Button.svelte';
	import Chip from '$lib/components/Chip.svelte';
	import Icon, { ICON_NAMES } from '$lib/components/Icon.svelte';
	import Input from '$lib/components/Input.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import SearchField from '$lib/components/SearchField.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import ThumbCard from '$lib/components/ThumbCard.svelte';
	import BackdropArt from '$lib/demo/BackdropArt.svelte';
	import PosterArt from '$lib/demo/PosterArt.svelte';
	import { dominant, films } from '$lib/demo/data';
	import { theme, THEMES } from '$lib/theme.svelte';
	import { toasts } from '$lib/toast.svelte';

	const colorTokens = [
		['--bg', 'Fond de l’application'],
		['--bg-sunken', 'Zones en retrait, champs'],
		['--surface', 'Cartes, panneaux'],
		['--surface-raised', 'Menus, modales'],
		['--line', 'Séparateurs'],
		['--text', 'Texte principal'],
		['--text-muted', 'Texte secondaire'],
		['--text-faint', 'Légendes, placeholders'],
		['--accent', 'Action, progression, focus'],
		['--accent-ink', 'Accent pour petit texte'],
		['--accent-contrast', 'Texte sur l’accent'],
		['--accent-soft', 'Fond teinté (sélection)'],
		['--danger', 'Erreur'],
		['--success', 'Succès']
	] as const;

	const typeScale = [
		{ token: '--fs-hero', spec: 'clamp(2.75 → 4.5rem) / 1.05', cls: 'hero', sample: 'Les Heures claires' },
		{ token: '--fs-h1', spec: '2.25rem / 1.15', cls: 'h1', sample: 'Bibliothèque de films' },
		{ token: '--fs-h2', spec: '1.5rem / 1.25', cls: 'h2', sample: 'Récemment ajoutés' },
		{ token: '--fs-h3', spec: '1.125rem / 1.35', cls: 'h3', sample: 'Marée basse' },
		{ token: '--fs-body', spec: '1rem / 1.6', cls: 'body', sample: 'Sur une île de la Manche, un gardien de phare à la retraite et une biologiste venue du continent attendent la plus grande marée du siècle.' },
		{ token: '--fs-small', spec: '0.875rem / 1.5', cls: 'small', sample: 'Réalisé par Jonas Lefèvre · VOSTFR · 4K HDR' },
		{ token: '--fs-micro', spec: '0.75rem / 1.4 · +0.06em · capitales', cls: 'micro', sample: 'Film · 2019 · 2 h 12' }
	];

	const spacing = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
	const radii = ['sm', 'md', 'lg', 'full'];
	const shadows = [1, 2, 3];
	const motion = [
		['--ease', 'cubic-bezier(.2, .7, .2, 1)', 'Sortie douce, jamais de rebond'],
		['--dur-fast', '150 ms', 'Survols, focus'],
		['--dur-base', '260 ms', 'Apparitions, menus'],
		['--dur-slow', '480 ms', 'Hero, fondus d’images']
	];
	const states = ['Repos', 'Survol', 'Focus', 'Désactivé'] as const;
	const demoAttr = (s: (typeof states)[number]) => (s === 'Survol' ? 'hover' : s === 'Focus' ? 'focus' : undefined);

	let values = $state<Record<string, string>>({});
	$effect(() => {
		theme.resolved;
		tick().then(() => {
			const cs = getComputedStyle(document.documentElement);
			values = Object.fromEntries(colorTokens.map(([t]) => [t, cs.getPropertyValue(t).trim()]));
		});
	});

	let genres = $state([
		{ label: 'Drame', selected: true, count: 48 },
		{ label: 'Documentaire', selected: false, count: 21 },
		{ label: 'Science-fiction', selected: true, count: 17 },
		{ label: 'Comédie', selected: false, count: 33 },
		{ label: 'Animation', selected: false, count: 9 },
		{ label: 'Non vus', selected: false, count: undefined }
	]);

	let filled = $state('Nord magnétique');
	let search = $state('');
	let reveal = $state(0);
	let played = $state(false);

	const sample = films.slice(0, 5);
</script>

<svelte:head>
	<title>Système de design — Aether</title>
</svelte:head>

<div class="page design">
	<header class="intro">
		<p class="micro eyebrow">Aether · Système de design · v0.1</p>
		<h1>Une salle de cinéma privée.</h1>
		<p class="lede">
			Velours, bois sombre, lumière tamisée. L’affiche est la star ; l’interface s’efface. Cette page rassemble les
			fondations et les composants, rendus en direct dans le thème actif.
		</p>
	</header>

	<!-- Thèmes -->
	<section class="block" aria-labelledby="s-themes">
		<div class="heading">
			<span class="num micro tabular">01</span>
			<h2 id="s-themes">Thèmes</h2>
			<p>Cinq ambiances sobres. Chaque aperçu est rendu avec ses propres tokens.</p>
		</div>
		<div class="themes">
			{#each THEMES as t (t.id)}
				{@const active = theme.resolved === t.id}
				<button type="button" class="theme-card" data-theme={t.id} class:active aria-pressed={active} onclick={() => theme.set(t.id)}>
					<span class="theme-preview" aria-hidden="true">
						<span class="tp-poster">
							<PosterArt title={films[1].title} credit={films[1].director} hue={films[1].hue} layout="horizon" />
						</span>
						<span class="tp-text">
							<span class="micro tp-meta">Film · 2021</span>
							<span class="tp-title">Marée basse</span>
							<span class="tp-muted">1 h 44 · VOSTFR</span>
							<span class="tp-actions">
								<span class="tp-btn">Lecture</span>
							</span>
						</span>
					</span>
					<span class="theme-foot">
						<span>
							<span class="theme-name">{t.label}</span>
							<span class="theme-mood">{t.mood}</span>
						</span>
						<span class="theme-state micro" class:on={active}>
							{#if active}<Icon name="check" size={14} />Actif{:else}Choisir{/if}
						</span>
					</span>
				</button>
			{/each}
		</div>
		<div class="row-inline">
			<Chip selected={theme.pref === 'system'} onclick={() => theme.set(theme.pref === 'system' ? theme.resolved : 'system')}>Suivre le système</Chip>
			<Chip selected={theme.grain} onclick={() => theme.setGrain(!theme.grain)}>Grain pellicule</Chip>
		</div>
	</section>

	<!-- Couleurs -->
	<section class="block" aria-labelledby="s-colors">
		<div class="heading">
			<span class="num micro tabular">02</span>
			<h2 id="s-colors">Couleurs</h2>
			<p>Tokens sémantiques. Un thème ne redéfinit que ces variables.</p>
		</div>
		<ul class="swatches" role="list">
			{#each colorTokens as [token, role] (token)}
				<li class="swatch">
					<span class="chip-color" style:background="var({token})"></span>
					<span class="sw-name">{token}</span>
					<span class="sw-role">{role}</span>
					<span class="sw-value tabular">{values[token] ?? ''}</span>
				</li>
			{/each}
		</ul>
	</section>

	<!-- Typographie -->
	<section class="block" aria-labelledby="s-type">
		<div class="heading">
			<span class="num micro tabular">03</span>
			<h2 id="s-type">Typographie</h2>
			<p>Cormorant Garamond pour l’affiche et le générique, Instrument Sans pour l’interface. Auto-hébergées.</p>
		</div>
		<div class="type-scale">
			{#each typeScale as t (t.token)}
				<div class="type-row">
					<div class="type-meta">
						<span class="sw-name">{t.token}</span>
						<span class="sw-value tabular">{t.spec}</span>
					</div>
					<p class="specimen {t.cls}">{t.sample}</p>
				</div>
			{/each}
			<div class="type-row">
				<div class="type-meta">
					<span class="sw-name">Italique serif</span>
					<span class="sw-value">citations, taglines</span>
				</div>
				<p class="specimen tagline">« Ce que la mer rend, elle le reprend. »</p>
			</div>
		</div>
	</section>

	<!-- Espace, rayons, ombres -->
	<section class="block" aria-labelledby="s-space">
		<div class="heading">
			<span class="num micro tabular">04</span>
			<h2 id="s-space">Espace, formes, matière</h2>
			<p>Base 4. Gouttières de 16, 32 puis 64 px. Ombres neutres, larges et diffuses.</p>
		</div>
		<div class="foundations">
			<div class="panel">
				<h3>Espacement</h3>
				<ul class="spacing" role="list">
					{#each spacing as n (n)}
						<li>
							<span class="sw-name">--sp-{n}</span>
							<span class="bar" style:width="var(--sp-{n})"></span>
						</li>
					{/each}
				</ul>
			</div>
			<div class="panel">
				<h3>Rayons</h3>
				<ul class="radii" role="list">
					{#each radii as r (r)}
						<li>
							<span class="radius-box" style:border-radius="var(--r-{r})"></span>
							<span class="sw-name">--r-{r}</span>
						</li>
					{/each}
				</ul>
				<h3 class="mt">Ombres</h3>
				<ul class="shadows" role="list">
					{#each shadows as s (s)}
						<li>
							<span class="shadow-box" style:box-shadow="var(--shadow-{s})"></span>
							<span class="sw-name">--shadow-{s}</span>
						</li>
					{/each}
				</ul>
			</div>
		</div>
	</section>

	<!-- Mouvement -->
	<section class="block" aria-labelledby="s-motion">
		<div class="heading">
			<span class="num micro tabular">05</span>
			<h2 id="s-motion">Mouvement</h2>
			<p>Feutré : fondus et légères translations. Avec « réduire les animations », seuls les fondus restent.</p>
		</div>
		<div class="motion">
			{#each motion as [token, value, use] (token)}
				<div class="motion-row">
					<span class="sw-name">{token}</span>
					<span class="sw-value tabular">{value}</span>
					<span class="sw-role">{use}</span>
					{#if token !== '--ease'}
						<span class="track"><span class="puck" class:played style:transition-duration="var({token})"></span></span>
					{:else}
						<span></span>
					{/if}
				</div>
			{/each}
			<div>
				<Button variant="tertiary" icon="play" onclick={() => (played = !played)}>Rejouer</Button>
			</div>
		</div>
	</section>

	<!-- Icônes -->
	<section class="block" aria-labelledby="s-icons">
		<div class="heading">
			<span class="num micro tabular">06</span>
			<h2 id="s-icons">Icônes</h2>
			<p>Trait de 1,5 px, coins arrondis, monochromes. Le plein est réservé aux états actifs.</p>
		</div>
		<ul class="icons" role="list">
			{#each ICON_NAMES as name (name)}
				<li><Icon {name} size={24} /><span>{name}</span></li>
			{/each}
			<li><Icon name="heart" size={24} filled /><span>heart · actif</span></li>
		</ul>
	</section>

	<!-- Boutons -->
	<section class="block" aria-labelledby="s-buttons">
		<div class="heading">
			<span class="num micro tabular">07</span>
			<h2 id="s-buttons">Boutons</h2>
			<p>Une seule action primaire par vue. Hauteurs 40 et 48 px.</p>
		</div>
		<div class="matrix">
			<span></span>
			{#each states as s (s)}<span class="micro col-head">{s}</span>{/each}
			{#each ['primary', 'secondary', 'tertiary'] as const as v (v)}
				<span class="micro row-head">{v === 'primary' ? 'Primaire' : v === 'secondary' ? 'Secondaire' : 'Tertiaire'}</span>
				{#each states as s (s)}
					<div>
						<Button variant={v} icon={v === 'primary' ? 'play' : undefined} iconFilled data-demo={demoAttr(s)} disabled={s === 'Désactivé'}>
							{v === 'primary' ? 'Lecture' : v === 'secondary' ? 'Plus d’infos' : 'Annuler'}
						</Button>
					</div>
				{/each}
			{/each}
		</div>
		<div class="row-inline">
			<Button variant="primary" size="lg" icon="play" iconFilled>Lecture</Button>
			<Button variant="secondary" size="lg" icon="plus">Ma liste</Button>
			<Button variant="tertiary" size="lg">Bande-annonce</Button>
			<Button variant="secondary" size="lg" round icon="heart" aria-label="Ajouter aux favoris" />
			<Button variant="secondary" round icon="sliders" aria-label="Filtres" />
			<Button variant="tertiary" round icon="x" aria-label="Fermer" />
		</div>
	</section>

	<!-- Chips et champs -->
	<section class="block" aria-labelledby="s-inputs">
		<div class="heading">
			<span class="num micro tabular">08</span>
			<h2 id="s-inputs">Filtres et champs</h2>
			<p>Champs en retrait, sans bordure au repos ; une ligne d’accent au focus.</p>
		</div>
		<div class="foundations">
			<div class="panel">
				<h3>Chips</h3>
				<div class="chips">
					{#each genres as g (g.label)}
						<Chip bind:selected={g.selected} count={g.count}>{g.label}</Chip>
					{/each}
				</div>
				<div class="chips states-row">
					<Chip>Repos</Chip>
					<Chip data-demo="hover">Survol</Chip>
					<Chip data-demo="focus">Focus</Chip>
					<Chip selected>Sélectionné</Chip>
					<Chip disabled>Désactivé</Chip>
				</div>
			</div>
			<div class="panel fields">
				<h3>Champs</h3>
				<SearchField bind:value={search} size="lg" label="Rechercher dans la bibliothèque" />
				<Input placeholder="Repos" aria-label="Exemple au repos" />
				<Input placeholder="Focus" aria-label="Exemple au focus" data-demo="focus" />
				<Input bind:value={filled} label="Titre" hint="Le titre original est conservé dans les métadonnées." />
				<Input placeholder="Désactivé" aria-label="Exemple désactivé" disabled />
			</div>
		</div>
	</section>

	<!-- Cartes -->
	<section class="block" aria-labelledby="s-cards">
		<div class="heading">
			<span class="num micro tabular">09</span>
			<h2 id="s-cards">Cartes</h2>
			<p>Affiche 2:3 et vignette 16:9. L’image apparaît en fondu depuis sa couleur dominante.</p>
		</div>
		{#key reveal}
			<div class="posters">
				{#each sample as f, i (f.id)}
					<PosterCard
						href="/design"
						title={f.title}
						year={f.year}
						progress={i === 1 ? 0.62 : i === 3 ? 0.18 : undefined}
						color={dominant(f.hue, f.sat)}
						data-demo={i === 2 ? 'hover' : undefined}
					>
						{#snippet art()}
							<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} sat={f.sat} layout={f.layout} />
						{/snippet}
					</PosterCard>
				{/each}
				<div class="loading">
					<Skeleton ratio="2 / 3" />
					<Skeleton lines={2} width="70%" />
				</div>
			</div>
		{/key}
		<div class="thumbs">
			<ThumbCard href="/design" eyebrow="Les Veilleurs · S2 · É4" title="Le Phare de Kéréon" detail="Reste 34 min" progress={0.35} color={dominant(205)}>
				{#snippet art()}<BackdropArt hue={205} scene="sea" />{/snippet}
			</ThumbCard>
			<ThumbCard href="/design" eyebrow="Hautes Terres · S1 · É7" title="Ce qui reste du feu" detail="Reste 15 min" progress={0.68} color={dominant(30)}>
				{#snippet art()}<BackdropArt hue={30} scene="ridges" />{/snippet}
			</ThumbCard>
			<div class="loading">
				<Skeleton ratio="16 / 9" />
				<Skeleton lines={2} width="80%" />
			</div>
		</div>
		<div class="row-inline">
			<Button variant="tertiary" icon="image" onclick={() => reveal++}>Rejouer le fondu</Button>
		</div>
	</section>

	<!-- Notifications -->
	<section class="block" aria-labelledby="s-toasts">
		<div class="heading">
			<span class="num micro tabular">10</span>
			<h2 id="s-toasts">Notifications</h2>
			<p>En bas à droite, sur surface relevée, sans icône colorée.</p>
		</div>
		<div class="row-inline">
			<Button variant="secondary" onclick={() => toasts.show('Bibliothèque « Films » analysée', { detail: '12 nouveaux titres' })}>
				Notification simple
			</Button>
			<Button
				variant="secondary"
				onclick={() =>
					toasts.show('Retiré de « Ma liste »', {
						action: { label: 'Annuler', run: () => toasts.show('« Marée basse » est de retour dans votre liste') }
					})}
			>
				Avec action
			</Button>
		</div>
	</section>
</div>

<style>
	.design {
		display: grid;
		gap: var(--sp-9);
	}

	.intro {
		max-width: 760px;
		padding-top: var(--sp-6);
	}

	.eyebrow {
		color: var(--accent-ink);
		margin-bottom: var(--sp-4);
	}

	.intro h1 {
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		letter-spacing: -0.015em;
	}

	.lede {
		margin-top: var(--sp-5);
		max-width: 60ch;
		color: var(--text-muted);
		font-size: 1.0625rem;
	}

	.block {
		display: grid;
		gap: var(--sp-6);
		min-width: 0;
	}

	.heading {
		display: grid;
		grid-template-columns: auto 1fr;
		column-gap: var(--sp-4);
		align-items: baseline;
		padding-top: var(--sp-5);
		border-top: 1px solid var(--line);
	}

	.heading h2 {
		font-size: var(--fs-h1);
		line-height: var(--lh-h1);
	}

	.heading p {
		grid-column: 2;
		color: var(--text-muted);
		max-width: 62ch;
		margin-top: var(--sp-2);
	}

	.num {
		color: var(--text-muted);
	}

	.row-inline {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
	}

	.sw-name {
		font-size: var(--fs-small);
		font-weight: 500;
		color: var(--text);
	}

	.sw-value,
	.sw-role {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.panel {
		min-width: 0;
		padding: var(--sp-6);
		background: var(--surface);
		border-radius: var(--r-md);
		box-shadow: var(--shadow-1);
	}

	.panel h3 {
		font-family: var(--font-ui);
		font-size: var(--fs-micro);
		letter-spacing: var(--ls-micro);
		text-transform: uppercase;
		font-weight: 500;
		color: var(--text-muted);
		margin-bottom: var(--sp-5);
	}

	.panel h3.mt {
		margin-top: var(--sp-7);
	}

	/* Themes */
	.themes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 184px), 1fr));
		gap: var(--sp-4);
	}

	.theme-card {
		display: block;
		width: 100%;
		text-align: left;
		border-radius: var(--r-md);
		overflow: hidden;
		box-shadow: var(--shadow-2), 0 0 0 1px var(--line);
		transition:
			box-shadow var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	.theme-card:hover {
		transform: translateY(-2px);
	}

	.theme-card:focus-visible {
		outline-offset: 3px;
	}

	.theme-card.active {
		box-shadow: var(--shadow-2), 0 0 0 1.5px var(--accent);
	}

	.theme-preview {
		display: flex;
		gap: var(--sp-3);
		padding: var(--sp-4);
		background: var(--bg);
	}

	.tp-poster {
		position: relative;
		flex: none;
		width: 56px;
		aspect-ratio: 2 / 3;
		border-radius: var(--r-sm);
		overflow: hidden;
		box-shadow: var(--shadow-1);
	}

	.tp-text {
		display: grid;
		align-content: center;
		gap: 2px;
		min-width: 0;
	}

	.tp-meta {
		color: var(--text-muted);
		font-size: 0.625rem;
	}

	.tp-title {
		font-family: var(--font-display);
		font-size: 1.1875rem;
		line-height: 1.15;
		color: var(--text);
		white-space: nowrap;
	}

	.tp-muted {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.tp-actions {
		display: flex;
		gap: 6px;
		margin-top: var(--sp-3);
	}

	.tp-btn {
		display: inline-grid;
		place-items: center;
		height: 24px;
		padding: 0 10px;
		border-radius: var(--r-sm);
		font-size: 0.6875rem;
		font-weight: 500;
	}

	.tp-btn {
		background: var(--accent);
		color: var(--accent-contrast);
	}

	.theme-foot {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4) var(--sp-4);
		background: var(--surface);
		border-top: 1px solid var(--line);
	}

	.theme-name,
	.theme-mood {
		display: block;
	}

	.theme-state {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		flex: none;
		color: var(--text-muted);
	}

	.theme-state.on {
		color: var(--accent-ink);
	}

	.theme-name {
		font-family: var(--font-display);
		font-size: var(--fs-h3);
		font-weight: 500;
		line-height: 1.2;
	}

	.theme-mood {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	/* Colours */
	.swatches {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr));
		gap: var(--sp-5) var(--sp-4);
	}

	.swatch {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.chip-color {
		height: 72px;
		margin-bottom: var(--sp-3);
		border-radius: var(--r-md);
		box-shadow: inset 0 0 0 1px var(--line), var(--shadow-1);
	}

	/* Type */
	.type-scale {
		display: grid;
	}

	.type-row {
		display: grid;
		grid-template-columns: 220px minmax(0, 1fr);
		gap: var(--sp-5);
		align-items: baseline;
		padding: var(--sp-5) 0;
		border-bottom: 1px solid var(--line);
	}

	.type-meta {
		display: grid;
		gap: 2px;
	}

	.specimen {
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.specimen.hero {
		font-family: var(--font-display);
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: 500;
		letter-spacing: -0.015em;
	}

	.specimen.h1,
	.specimen.h2,
	.specimen.h3 {
		font-family: var(--font-display);
		font-weight: 500;
	}

	.specimen.h1 {
		font-size: var(--fs-h1);
		line-height: var(--lh-h1);
	}

	.specimen.h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
	}

	.specimen.h3 {
		font-size: var(--fs-h3);
		line-height: var(--lh-h3);
	}

	.specimen.body {
		max-width: 66ch;
	}

	.specimen.small {
		font-size: var(--fs-small);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}

	.specimen.micro {
		color: var(--text-muted);
	}

	.specimen.tagline {
		font-family: var(--font-display);
		font-style: italic;
		font-size: 1.5rem;
		line-height: 1.3;
	}

	/* Foundations */
	.foundations {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
		gap: var(--sp-5);
	}

	.spacing {
		display: grid;
		gap: var(--sp-3);
	}

	.spacing li {
		display: grid;
		grid-template-columns: 64px 1fr;
		align-items: center;
		gap: var(--sp-4);
	}

	.bar {
		height: 8px;
		border-radius: 2px;
		background: var(--accent);
		opacity: 0.8;
	}

	.radii,
	.shadows {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-6);
	}

	.radii li,
	.shadows li {
		display: grid;
		justify-items: start;
		gap: var(--sp-3);
	}

	.radius-box {
		width: 64px;
		height: 64px;
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.shadow-box {
		width: 88px;
		height: 64px;
		border-radius: var(--r-md);
		background: var(--surface-raised);
	}

	/* Motion */
	.motion {
		display: grid;
		gap: var(--sp-3);
	}

	.motion-row {
		display: grid;
		grid-template-columns: 120px 180px minmax(0, 1fr) minmax(0, 1.2fr);
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-3) 0;
		border-bottom: 1px solid var(--line);
	}

	.track {
		position: relative;
		height: 2px;
		background: var(--line);
	}

	.puck {
		position: absolute;
		top: -5px;
		left: 0;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background: var(--accent);
		transition-property: left;
		transition-timing-function: var(--ease);
	}

	.puck.played {
		left: calc(100% - 12px);
	}

	/* Icons */
	.icons {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(112px, 1fr));
		gap: var(--sp-2);
	}

	.icons li {
		display: grid;
		justify-items: center;
		gap: var(--sp-3);
		padding: var(--sp-5) var(--sp-2) var(--sp-4);
		border-radius: var(--r-md);
		background: var(--surface);
		color: var(--text);
	}

	.icons span {
		font-size: var(--fs-micro);
		color: var(--text-muted);
		text-align: center;
	}

	/* Buttons */
	.matrix {
		display: grid;
		grid-template-columns: 120px repeat(4, minmax(0, max-content));
		gap: var(--sp-4) var(--sp-6);
		align-items: center;
		overflow-x: auto;
		padding: var(--sp-1);
	}

	.col-head,
	.row-head {
		color: var(--text-muted);
	}

	/* Chips & fields */
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.states-row {
		margin-top: var(--sp-6);
		padding-top: var(--sp-5);
		border-top: 1px solid var(--line);
	}

	.fields {
		display: grid;
		gap: var(--sp-4);
		align-content: start;
	}

	.fields h3 {
		margin-bottom: var(--sp-1);
	}

	/* Cards */
	.posters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(46%, 150px), 1fr));
		gap: var(--sp-6) var(--sp-5);
	}

	.thumbs {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 280px), 1fr));
		gap: var(--sp-6) var(--sp-5);
	}

	.loading {
		display: grid;
		gap: var(--sp-3);
		align-content: start;
	}

	@media (max-width: 767px) {
		.design {
			gap: var(--sp-8);
		}

		.heading {
			grid-template-columns: 1fr;
		}

		.heading p {
			grid-column: 1;
		}

		.num {
			margin-bottom: var(--sp-2);
		}

		.type-row {
			grid-template-columns: 1fr;
			gap: var(--sp-2);
		}

		.motion-row {
			grid-template-columns: 1fr 1fr;
			row-gap: var(--sp-2);
		}

		.panel {
			padding: var(--sp-5);
		}

		.matrix {
			grid-template-columns: 96px repeat(4, max-content);
		}
	}
</style>
