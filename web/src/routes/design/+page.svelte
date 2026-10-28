<script lang="ts">
	import { tick } from 'svelte';
	import Avatar from '$lib/components/Avatar.svelte';
	import Logo from '$lib/components/brand/Logo.svelte';
	import Button from '$lib/components/Button.svelte';
	import CategoryTile from '$lib/components/CategoryTile.svelte';
	import Chip from '$lib/components/Chip.svelte';
	import ContinueTile from '$lib/components/ContinueTile.svelte';
	import Icon, { ICON_NAMES } from '$lib/components/Icon.svelte';
	import IconButton from '$lib/components/IconButton.svelte';
	import Input from '$lib/components/Input.svelte';
	import PosterCard from '$lib/components/PosterCard.svelte';
	import SearchField from '$lib/components/SearchField.svelte';
	import SectionHeader from '$lib/components/SectionHeader.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Tag from '$lib/components/Tag.svelte';
	import ThumbCard from '$lib/components/ThumbCard.svelte';
	import UserChip from '$lib/components/UserChip.svelte';
	import Artwork from '$lib/components/Artwork.svelte';
	import Breadcrumbs from '$lib/components/Breadcrumbs.svelte';
	import Dialog from '$lib/components/Dialog.svelte';
	import EmptyState from '$lib/components/EmptyState.svelte';
	import Menu from '$lib/components/Menu.svelte';
	import Select from '$lib/components/Select.svelte';
	import TitleArt from '$lib/components/TitleArt.svelte';
	import type { Image, Source } from '$lib/api/types';
	import OfflineBadge from '$lib/components/OfflineBadge.svelte';
	import AgentHelp from '$lib/components/sources/AgentHelp.svelte';
	import SecretField from '$lib/components/sources/SecretField.svelte';
	import SourceForm from '$lib/components/sources/SourceForm.svelte';
	import SourceKindTiles from '$lib/components/sources/SourceKindTiles.svelte';
	import SourceRow from '$lib/components/sources/SourceRow.svelte';
	import SourceStatusPill from '$lib/components/sources/SourceStatusPill.svelte';
	import FolderPicker from '$lib/components/FolderPicker.svelte';
	import Stepper from '$lib/components/Stepper.svelte';
	import type { FolderListing } from '$lib/api/types';
	import { emptyDraft, sourceKindInfo, testKey, type SourceDraft } from '$lib/sources';
	import BackdropArt from '$lib/demo/BackdropArt.svelte';
	import PosterArt from '$lib/demo/PosterArt.svelte';
	import { currentUser, dominant, films, genres } from '$lib/demo/data';
	import { theme, THEMES } from '$lib/theme.svelte';
	import WatchBadge from '$lib/components/WatchBadge.svelte';
	import Switch from '$lib/components/Switch.svelte';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import PasswordStrength from '$lib/components/auth/PasswordStrength.svelte';
	import UserMenu from '$lib/components/shell/UserMenu.svelte';
	import CollectionCard from '$lib/components/collections/CollectionCard.svelte';
	import ProgressLine from '$lib/components/ProgressLine.svelte';
	import { toasts } from '$lib/toast.svelte';
	import CopyCode from '$lib/components/sync/CopyCode.svelte';
	import DeviceCodePanel from '$lib/components/sync/DeviceCodePanel.svelte';
	import ServiceMark from '$lib/components/sync/ServiceMark.svelte';
	import SyncJournal from '$lib/components/sync/SyncJournal.svelte';
	import SubNav from '$lib/components/shell/SubNav.svelte';
	import type { SyncEvent } from '$lib/api/types';

	const colorTokens = [
		['--bg', 'Fond de l’application'],
		['--bg-sunken', 'Zones en retrait'],
		['--surface', 'Cartes, tuiles, champs'],
		['--surface-raised', 'Menus, survols'],
		['--surface-active', 'Navigation active'],
		['--line', 'Séparateurs, contours'],
		['--text', 'Texte principal'],
		['--text-muted', 'Texte secondaire'],
		['--text-faint', 'Placeholders'],
		['--accent', 'Action, progression, focus'],
		['--accent-ink', 'Accent pour petit texte'],
		['--accent-contrast', 'Texte sur l’accent'],
		['--accent-soft', 'Fond teinté (sélection)'],
		['--danger', 'Erreur'],
		['--success', 'Succès']
	] as const;

	const typeScale = [
		{ token: '--fs-hero', spec: '30 → 40 px / 112 % · ExtraBold', cls: 'hero', sample: 'Les Heures claires' },
		{ token: '--fs-h1', spec: 'Heading · 40 px / 120 % · Bold', cls: 'h1', sample: 'Bibliothèque de films' },
		{ token: '--fs-h2', spec: 'Title · 20 px / 135 % · Bold', cls: 'h2', sample: 'Reprendre la lecture' },
		{ token: '--fs-h3', spec: '16 px / 140 % · Bold', cls: 'h3', sample: 'Marée basse' },
		{ token: '--fs-body', spec: 'Body · 14 px / 150 %', cls: 'body', sample: 'Sur une île de la Manche, un gardien de phare à la retraite et une biologiste venue du continent attendent la plus grande marée du siècle.' },
		{ token: '--fs-small', spec: '13 px / 145 %', cls: 'small', sample: 'Réalisé par Jonas Lefèvre · VOSTFR · 4K HDR' },
		{ token: '--fs-micro', spec: '12 px / 135 % · SemiBold', cls: 'micro', sample: 'Film · 2019 · 2 h 12' }
	];

	const spacing = [1, 2, 3, 4, 5, 6, 7, 8];
	const radii = [
		['xs', '6 px', 'badges'],
		['sm', '10 px', 'tuiles, nav'],
		['md', '12 px', 'champs, icônes'],
		['lg', '16 px', 'cartes'],
		['xl', '20 px', 'panneaux, hero'],
		['full', '∞', 'pilules']
	];
	const shadows = [1, 2, 3];
	const motion = [
		['--ease', 'cubic-bezier(.22, .8, .26, 1)', 'Sortie douce, jamais de rebond'],
		['--dur-fast', '140 ms', 'Survols, focus'],
		['--dur-base', '220 ms', 'Apparitions, menus'],
		['--dur-slow', '420 ms', 'Hero, fondus d’images']
	];
	const states = ['Repos', 'Survol', 'Focus', 'Désactivé'] as const;
	const demoAttr = (s: (typeof states)[number]) => (s === 'Survol' ? 'hover' : s === 'Focus' ? 'focus' : undefined);
	const toolIcons = ['search', 'heart', 'home', 'genres', 'clock', 'download', 'layers', 'bell', 'monitor', 'settings', 'compass', 'play'] as const;

	let values = $state<Record<string, string>>({});
	$effect(() => {
		theme.resolved;
		tick().then(() => {
			const cs = getComputedStyle(document.documentElement);
			values = Object.fromEntries(colorTokens.map(([t]) => [t, cs.getPropertyValue(t).trim()]));
		});
	});

	let filters = $state([
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

	// M4 components. An image without URL shows its decoded blurhash alone.
	const blurOnly: Image = { url: '', blurhash: 'T74of_t7M{X=M{j@XUt8ogbct7of', dominant: '#233953', width: 600, height: 900 };
	let sortValue = $state('title');
	let dialogOpen = $state(false);

	// M5b sources: static examples (no API).
	const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
	const demoSources: Source[] = [
		{ id: 1, kind: 'local', name: '/srv/media/films', root: '/srv/media/films', status: 'online', lastSeen: ago(0) },
		{ id: 2, kind: 'agent', name: 'Salon · films', root: '4K', status: 'online', lastSeen: ago(1), agent: { url: 'http://salon.local:8097', share: 'films', hostname: 'salon', version: '0.6.0' } },
		{ id: 3, kind: 'sftp', name: 'Seedbox', root: '/home/moi/downloads', status: 'offline', lastError: 'timeout', lastSeen: ago(42), sftp: { host: 'sb.example.net', port: 22, user: 'moi', auth: 'key', hostKey: 'SHA256:3f9K…' } }
	];
	// M6b synchronisation: static examples (no API).
	let syncNow = $state(Date.now());
	$effect(() => {
		const t = setInterval(() => (syncNow = Date.now()), 1000);
		return () => clearInterval(t);
	});
	const demoCode = { userCode: 'A1B2C3D4', verificationUrl: 'https://trakt.tv/activate', expiresAt: new Date(Date.now() + 7 * 60_000).toISOString(), interval: 5 };
	const demoExpired = { ...demoCode, userCode: 'Q7RT9XKP', expiresAt: ago(1) };
	const demoEvents: SyncEvent[] = [
		{ id: 5, time: ago(3), kind: 'conflict', feature: 'history', itemId: 1, itemTitle: 'Nord magnétique', winner: 'remote', message: 'Vu sur Trakt après avoir été marqué non vu ici.' },
		{ id: 4, time: ago(3), kind: 'import', feature: 'watchlist', itemId: 2, itemTitle: 'Marée basse', message: 'Ajouté à Ma liste.' },
		{ id: 3, time: ago(35), kind: 'export', feature: 'ratings', itemId: 3, itemTitle: 'Les Veilleurs', message: 'Note 8/10 envoyée.' },
		{ id: 2, time: ago(64), kind: 'conflict', feature: 'progress', itemId: 4, itemTitle: 'Lisière', winner: 'local', message: 'Reprise à 42 min conservée (plus récente).' },
		{ id: 1, time: ago(190), kind: 'error', feature: 'history', message: 'Trakt ne répond pas (503) ; nouvel essai dans 30 minutes.' }
	];
	let secretDemo = $state('s3cr3t-jeton');
	let pickedKind = $state<string | null>(null);
	function trustDraft(): SourceDraft {
		const d = emptyDraft('sftp');
		d.sftp.host = 'sb.example.net';
		d.sftp.user = 'moi';
		d.root = '/home/moi/downloads';
		d.sftp.password = 'secret';
		d.test = { ok: false, error: 'host_key_unknown', hostKey: 'SHA256:p2cIu6X0Qa+9u0Zf1kHq7lYgq8JxV3mWbQyQe4mJt2c', key: testKey(d) };
		return d;
	}
	let trustDemo = $state<SourceDraft>(trustDraft());
	function agentDraft(): SourceDraft {
		const d = emptyDraft('agent');
		d.agent.mode = 'token';
		d.agent.url = 'http://salon.local:8097';
		d.agent.token = 'Zk3q…';
		d.shares = ['films', 'series', 'concerts'];
		d.agent.share = 'films';
		d.test = { ok: true, agent: { hostname: 'salon', version: '0.6.0', shares: d.shares }, key: testKey(d) };
		return d;
	}
	let agentDemo = $state<SourceDraft>(agentDraft());

	// M5c: guided agent pairing — Stepper and FolderPicker on a static tree (no API).
	let stepDemo = $state(2);
	const demoTree: Record<string, string[]> = {
		'': ['/srv', '/mnt', '/home/camille'],
		'/srv': ['media', 'backups'],
		'/srv/media': ['Films', 'Series', 'Musique', 'Concerts'],
		'/srv/media/Films': ['4K', 'Classiques'],
		'/mnt': ['nas'],
		'/mnt/nas': ['Photos', 'Videos'],
		'/home/camille': ['Téléchargements', 'Vidéos']
	};
	async function demoLoad(path: string): Promise<FolderListing> {
		await new Promise((r) => setTimeout(r, 180));
		if (path === '/srv/backups') throw new Error('denied');
		const names = demoTree[path] ?? [];
		const dirs = names.map((n) => (path ? { name: n, path: `${path}/${n}` } : { name: n.split('/').pop() ?? n, path: n }));
		const parent = path && !demoTree[''].includes(path) ? path.slice(0, path.lastIndexOf('/')) : undefined;
		return { path, parent, dirs };
	}
	let demoFolder = $state('');
</script>

<svelte:head>
	<title>Système de design — Aether</title>
</svelte:head>

<div class="page design">
	<header class="intro panel">
		<div class="intro-copy">
			<p class="eyebrow">Aether · Système de design · v0.2 « Nuit »</p>
			<h1>Bleu nuit, sarcelle mate, formes douces.</h1>
			<p class="lede">
				Une seule famille arrondie (Nunito), des tuiles généreuses, des panneaux teintés au coin. L’affiche reste la
				star ; l’accent est réservé à l’action et à la progression. Tout est rendu en direct dans le thème actif.
			</p>
		</div>
		<div class="intro-mark" aria-hidden="true"><Logo size={88} wordmark={false} /></div>
	</header>

	<!-- Thèmes -->
	<section class="block" aria-labelledby="s-themes">
		<div class="heading">
			<span class="num tabular">01</span>
			<h2 id="s-themes">Thèmes</h2>
			<p>Nuit par défaut, cinq variantes. Chaque aperçu est rendu avec ses propres tokens. « Système » suit Nuit ↔ Lin.</p>
		</div>
		<div class="themes">
			{#each THEMES as t (t.id)}
				{@const active = theme.resolved === t.id}
				<button type="button" class="theme-card" data-theme={t.id} class:active aria-pressed={active} onclick={() => theme.set(t.id)}>
					<span class="tp" aria-hidden="true">
						<span class="tp-side">
							<span class="tp-logo"><Logo size={14} wordmark={false} /></span>
							<span class="tp-nav on"></span>
							<span class="tp-nav"></span>
							<span class="tp-nav"></span>
						</span>
						<span class="tp-main">
							<span class="tp-hero">
								<BackdropArt hue={films[1].hue} hue2={films[1].hue2} scene="aurora" focus={0.7} />
								<span class="tp-hero-copy">
									<span class="tp-title">Nord magnétique</span>
									<span class="tp-btn"><Icon name="play" size={8} filled />Lecture</span>
								</span>
							</span>
							<span class="tp-panel">
								<span class="tp-line w60"></span>
								<span class="tp-line w40 muted"></span>
								<span class="tp-bar"><span></span></span>
							</span>
						</span>
					</span>
					<span class="theme-foot">
						<span>
							<span class="theme-name">{t.label}</span>
							<span class="theme-mood">{t.mood}</span>
						</span>
						<span class="theme-state" class:on={active}>
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
			<span class="num tabular">02</span>
			<h2 id="s-colors">Couleurs</h2>
			<p>Tokens sémantiques. Un thème ne redéfinit que ces variables, plus la teinte de coin des panneaux.</p>
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
			<span class="num tabular">03</span>
			<h2 id="s-type">Typographie</h2>
			<p>Nunito partout, auto-hébergée (variable 200–1000). Heading 40, Title 20, Body 14 — et un titre d’affiche plus large.</p>
		</div>
		<div class="type-scale panel">
			{#each typeScale as t (t.token)}
				<div class="type-row">
					<div class="type-meta">
						<span class="sw-name">{t.token}</span>
						<span class="sw-value tabular">{t.spec}</span>
					</div>
					<p class="specimen {t.cls}">{t.sample}</p>
				</div>
			{/each}
		</div>
	</section>

	<!-- Espace, rayons, ombres -->
	<section class="block" aria-labelledby="s-space">
		<div class="heading">
			<span class="num tabular">04</span>
			<h2 id="s-space">Espace, formes, matière</h2>
			<p>Base 4. Gouttières 16 / 24 / 28 px, espacement entre cartes 12–16 px. Rayons généreux, ombres profondes et diffuses.</p>
		</div>
		<div class="foundations">
			<div class="card panel">
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
			<div class="card panel">
				<h3>Rayons</h3>
				<ul class="radii" role="list">
					{#each radii as [r, px, use] (r)}
						<li>
							<span class="radius-box" style:border-radius="var(--r-{r})"></span>
							<span class="sw-name">--r-{r}</span>
							<span class="sw-role">{px} · {use}</span>
						</li>
					{/each}
				</ul>
				<h3 class="mt">Ombres et teinte de panneau</h3>
				<ul class="shadows" role="list">
					{#each shadows as s (s)}
						<li>
							<span class="shadow-box" style:box-shadow="var(--shadow-{s})"></span>
							<span class="sw-name">--shadow-{s}</span>
						</li>
					{/each}
					<li>
						<span class="shadow-box tinted"></span>
						<span class="sw-name">--tint</span>
					</li>
				</ul>
			</div>
		</div>
	</section>

	<!-- Mouvement -->
	<section class="block" aria-labelledby="s-motion">
		<div class="heading">
			<span class="num tabular">05</span>
			<h2 id="s-motion">Mouvement</h2>
			<p>Court et fluide : fondus et légères translations. Avec « réduire les animations », seuls les fondus restent.</p>
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

	<!-- Marque et icônes -->
	<section class="block" aria-labelledby="s-icons">
		<div class="heading">
			<span class="num tabular">06</span>
			<h2 id="s-icons">Marque et icônes</h2>
			<p>Monogramme « A » plié en deux facettes. Icônes au trait de 1,5 px ; en tuile carrée arrondie quand elles sont actionnables.</p>
		</div>
		<div class="foundations">
			<div class="card panel brand">
				<Logo size={56} />
				<div class="brand-sizes">
					<span class="app-icon lg"><Logo size={44} wordmark={false} /></span>
					<span class="app-icon md"><Logo size={30} wordmark={false} /></span>
					<span class="app-icon sm"><Logo size={18} wordmark={false} /></span>
				</div>
			</div>
			<div class="card panel">
				<h3>Tuiles d’icônes</h3>
				<div class="tool-grid">
					{#each toolIcons as name (name)}
						<IconButton icon={name} size="lg" variant={name === 'home' ? 'accent' : 'surface'} filled={name === 'home'} aria-label={name} />
					{/each}
				</div>
			</div>
		</div>
		<ul class="icons" role="list">
			{#each ICON_NAMES as name (name)}
				<li><Icon {name} size={22} /><span>{name}</span></li>
			{/each}
		</ul>
	</section>

	<!-- Boutons -->
	<section class="block" aria-labelledby="s-buttons">
		<div class="heading">
			<span class="num tabular">07</span>
			<h2 id="s-buttons">Boutons</h2>
			<p>Pilules. Une seule action primaire par vue (sarcelle pleine, encre sombre). Hauteurs 40 et 46 px.</p>
		</div>
		<div class="matrix panel">
			<span></span>
			{#each states as s (s)}<span class="col-head">{s}</span>{/each}
			{#each ['primary', 'secondary', 'tertiary'] as const as v (v)}
				<span class="row-head">{v === 'primary' ? 'Primaire' : v === 'secondary' ? 'Secondaire' : 'Tertiaire'}</span>
				{#each states as s (s)}
					<div>
						<Button variant={v} icon={v === 'primary' ? 'play' : undefined} iconFilled data-demo={demoAttr(s)} disabled={s === 'Désactivé'}>
							{v === 'primary' ? 'Lecture' : v === 'secondary' ? 'Plus d’infos' : 'Annuler'}
						</Button>
					</div>
				{/each}
			{/each}
			<span class="row-head">Icône</span>
			{#each states as s (s)}
				<div>
					<IconButton icon="bell" aria-label="Notifications" data-demo={demoAttr(s)} disabled={s === 'Désactivé'} badge={s === 'Repos'} />
				</div>
			{/each}
		</div>
		<div class="row-inline">
			<Button variant="primary" size="lg" icon="play" iconFilled>Lecture</Button>
			<Button variant="secondary" size="lg" icon="plus">Ma liste</Button>
			<Button variant="tertiary" size="lg">Bande-annonce</Button>
			<IconButton icon="heart" size="lg" aria-label="Ajouter aux favoris" />
			<IconButton icon="sliders" aria-label="Filtres" />
			<IconButton icon="x" variant="ghost" aria-label="Fermer" />
		</div>
	</section>

	<!-- Chips et champs -->
	<section class="block" aria-labelledby="s-inputs">
		<div class="heading">
			<span class="num tabular">08</span>
			<h2 id="s-inputs">Filtres, étiquettes et champs</h2>
			<p>Champs sur surface, contour discret ; anneau sarcelle mat au focus.</p>
		</div>
		<div class="foundations">
			<div class="card panel">
				<h3>Chips de filtre</h3>
				<div class="chips">
					{#each filters as g (g.label)}
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
				<h3 class="mt">Étiquettes</h3>
				<div class="chips">
					<Tag>4K HDR</Tag>
					<Tag variant="accent">Nouveau</Tag>
					<span class="tag-on-image">
						<BackdropArt hue={286} hue2={236} scene="forest" />
						<Tag variant="image">Animation</Tag>
						<Tag variant="image" icon="trending">Tendance</Tag>
					</span>
				</div>
			</div>
			<div class="card panel fields">
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
			<span class="num tabular">09</span>
			<h2 id="s-cards">Cartes et tuiles</h2>
			<p>Affiche 2:3, vignette 16:9, tuile « Reprendre » avec temps écoulé / durée, tuile de catégorie.</p>
		</div>

		<SectionHeader title="Titre de section" href="/design" />

		{#key reveal}
			<div class="posters">
				{#each sample as f, i (f.id)}
					<PosterCard href="/design" title={f.title} year={f.year} progress={i === 1 ? 0.62 : undefined} color={dominant(f.hue, f.hue2)} data-demo={i === 2 ? 'hover' : undefined}>
						{#snippet art()}
							<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />
						{/snippet}
					</PosterCard>
				{/each}
				<div class="loading">
					<Skeleton ratio="2 / 3" radius="lg" />
					<Skeleton lines={2} width="70%" />
				</div>
			</div>
		{/key}

		<div class="continue-demo">
			<div class="ct-featured">
				<ContinueTile featured href="/design" title="Les Veilleurs" subtitle="S2 · É4 · Le Phare de Kéréon" elapsed="57:36" total="1:30:00" progress={0.64} color={dominant(278, 232)}>
					{#snippet art()}<BackdropArt hue={278} hue2={232} scene="city" focus={0.55} />{/snippet}
				</ContinueTile>
			</div>
			<ContinueTile href="/design" title="Archipel" elapsed="24:56" total="29:00" progress={0.86} color={dominant(196, 238)}>
				{#snippet art()}<BackdropArt hue={196} hue2={238} scene="planet" focus={0.5} />{/snippet}
			</ContinueTile>
			<ContinueTile href="/design" title="Hautes Terres" elapsed="22:48" total="1:00:00" progress={0.38} color={dominant(120, 210)} data-demo="hover">
				{#snippet art()}<BackdropArt hue={120} hue2={210} scene="forest" focus={0.5} />{/snippet}
			</ContinueTile>
		</div>

		<div class="thumbs">
			<ThumbCard href="/design" eyebrow="Drame · 2021" title="Marée basse" detail="1 h 44" progress={0.35} color={dominant(188, 226)}>
				{#snippet art()}<BackdropArt hue={188} hue2={226} scene="sea" />{/snippet}
			</ThumbCard>
			<ThumbCard href="/design" eyebrow="Science-fiction · 2023" title="Atlas" detail="2 h 36" color={dominant(198, 266)} data-demo="hover">
				{#snippet art()}<BackdropArt hue={198} hue2={266} scene="planet" />{/snippet}
			</ThumbCard>
			<div class="loading">
				<Skeleton ratio="16 / 9" radius="lg" />
				<Skeleton lines={2} width="80%" />
			</div>
		</div>

		<div class="cats">
			{#each genres.slice(0, 4) as g, i (g.id)}
				<CategoryTile label={g.label} href="/design" color={dominant(g.hue, g.hue2)} data-demo={i === 1 ? 'hover' : undefined}>
					{#snippet art()}<BackdropArt hue={g.hue} hue2={g.hue2} scene={g.scene} focus={0.5} />{/snippet}
				</CategoryTile>
			{/each}
		</div>

		<div class="row-inline">
			<UserChip name={currentUser.name} detail={currentUser.email} hue={currentUser.hue} href="/design" />
			<Avatar name="Noé Martin" hue={28} size={40} />
			<Avatar name="Inès Moreau" hue={300} size={32} />
			<Button variant="tertiary" icon="image" onclick={() => reveal++}>Rejouer le fondu</Button>
		</div>
	</section>

	<!-- M4 : parcourir -->
	<section class="block" aria-labelledby="s-browse">
		<div class="heading">
			<span class="num tabular">11</span>
			<h2 id="s-browse">Parcourir (M4)</h2>
			<p>Images réelles : couleur dominante, puis blurhash décodé, puis fondu de l’image. Sans image, le titre est composé sur une tuile teintée.</p>
		</div>
		<div class="m4-art">
			<figure>
				<div class="m4-frame"><Artwork image={blurOnly} ratio="2 / 3" /></div>
				<figcaption class="micro">Blurhash</figcaption>
			</figure>
			<figure>
				<div class="m4-frame"><Artwork ratio="2 / 3" placeholder={{ title: 'Les Heures claires', year: 2019, icon: 'film' }} /></div>
				<figcaption class="micro">Sans affiche</figcaption>
			</figure>
			<figure>
				<div class="m4-frame square"><Artwork ratio="1 / 1" placeholder={{ title: 'Nocturnes', icon: 'disc' }} /></div>
				<figcaption class="micro">Album</figcaption>
			</figure>
			<figure>
				<div class="m4-frame round"><Artwork ratio="1 / 1" placeholder={{ title: 'Lune Pâle' }} /></div>
				<figcaption class="micro">Artiste</figcaption>
			</figure>
			<figure class="wide">
				<div class="m4-frame wide"><div class="m4-wide"><TitleArt title="Grande Marée" year="S2 · É3" icon="tv" layout="wide" /></div></div>
				<figcaption class="micro">Vignette d’épisode</figcaption>
			</figure>
		</div>
		<div class="row-inline">
			<Breadcrumbs items={[{ label: 'Séries', href: '/design' }, { label: 'Les Veilleurs', href: '/design' }, { label: 'Saison 1' }]} />
		</div>
		<div class="row-inline">
			<div style="width: 200px"><Select label="Trier par" bind:value={sortValue} options={[{ value: 'title', label: 'Titre' }, { value: 'dateAdded', label: 'Date d’ajout' }, { value: 'year', label: 'Année' }]} /></div>
			<Menu
				label="Plus d’actions"
				text="Plus"
				items={[
					{ label: 'Actualiser les métadonnées', icon: 'refresh', onselect: () => toasts.show('Actualisation demandée') },
					{ label: 'Identifier…', icon: 'wand', onselect: () => toasts.show('Identifier') },
					{ label: 'Modifier…', icon: 'edit', onselect: () => toasts.show('Modifier') }
				]}
			/>
			<Button variant="primary" icon="play" iconFilled aria-disabled="true" title="Lecture disponible en M5">Lecture</Button>
			<Button variant="secondary" onclick={() => (dialogOpen = true)}>Ouvrir une boîte de dialogue</Button>
			<Tag variant="accent" icon="wand">À identifier</Tag>
		</div>
		<EmptyState icon="search" title="Aucun résultat pour « zzz »" compact>
			<p>Vérifiez l’orthographe, essayez un autre mot-clé ou le titre original.</p>
		</EmptyState>
		<Dialog bind:open={dialogOpen} title="Boîte de dialogue" description="Focus piégé, Échap pour fermer, retour du focus au déclencheur." size="sm">
			<p class="m4-dialog-text">Le fond de page est inerte tant que la boîte est ouverte.</p>
			{#snippet footer()}
				<Button variant="tertiary" onclick={() => (dialogOpen = false)}>Annuler</Button>
				<Button variant="primary" onclick={() => (dialogOpen = false)}>Valider</Button>
			{/snippet}
		</Dialog>
	</section>

	<!-- M5b : sources -->
	<section class="block" aria-labelledby="s-sources">
		<div class="heading">
			<span class="num tabular">12</span>
			<h2 id="s-sources">Sources (M5b)</h2>
			<p>Une bibliothèque regroupe des sources : dossier local, Agent Aether, SFTP. Tuiles de choix navigables au clavier (Tab, flèches), pastilles d’état et titres hors ligne grisés.</p>
		</div>
		<SourceKindTiles onpick={(k) => (pickedKind = sourceKindInfo(k).label)} demo="agent" />
		{#if pickedKind}<p class="micro m5b-note" role="status">Choisi : {pickedKind}</p>{/if}
		<div class="row-inline">
			<SourceStatusPill status="online" lastSeen={ago(0)} />
			<SourceStatusPill status="offline" lastError="connection_refused" lastSeen={ago(12)} />
			<SourceStatusPill status="unknown" />
			<OfflineBadge variant="surface" />
		</div>
		<ul class="m5b-sources" role="list">
			{#each demoSources as src (src.id)}<li><SourceRow source={src} onedit={() => toasts.show(`Modifier « ${src.name} »`)} ondelete={() => toasts.show(`Supprimer « ${src.name} »`)} /></li>{/each}
		</ul>
		<div class="m5b-grid">
			<div class="m5b-panel panel">
				<p class="micro m5b-cap">Agent à jeton manuel (agent.toml) après un test réussi</p>
				<SourceForm bind:draft={agentDemo} idp="demo-agent" />
			</div>
			<div class="m5b-panel panel">
				<p class="micro m5b-cap">SFTP : empreinte à confirmer</p>
				<SourceForm bind:draft={trustDemo} idp="demo-sftp" />
			</div>
		</div>
		<div class="m5b-grid">
			<div><SecretField id="demo-secret" label="Jeton d’appairage" bind:value={secretDemo} hint="Champ secret : masqué, bouton Afficher / Masquer." /></div>
			<AgentHelp />
		</div>
		<div class="m5b-cards">
			<PosterCard title={films[0].title} year={films[0].year} color={dominant(films[0].hue, films[0].hue2)} href="/design">
				{#snippet art()}{@const f = films[0]}<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />{/snippet}
			</PosterCard>
			<PosterCard title={films[1].title} year={films[1].year} color={dominant(films[1].hue, films[1].hue2)} href="/design" offline>
				{#snippet art()}{@const f = films[1]}<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />{/snippet}
			</PosterCard>
			<Button variant="primary" icon="play" iconFilled aria-disabled="true" title="Source hors ligne">Lecture</Button>
		</div>
	</section>

	<!-- Comptes et progression (M6) -->
	<section class="block" aria-labelledby="s-m6">
		<div class="heading">
			<span class="num tabular">13</span>
			<h2 id="s-m6">Comptes et progression (M6)</h2>
			<p>Carte de connexion, menu utilisateur, badges de visionnage (« Vu », épisodes non vus, progression), interrupteurs et mot de passe avec indice de robustesse.</p>
		</div>
		<div class="m6-grid">
			<div class="m6-login panel" aria-label="Aperçu de la carte de connexion">
				<p class="m6-title">Connexion</p>
				<div class="m6-alert" role="presentation"><Icon name="alert" size={18} /><span><strong>Identifiants incorrects</strong><br />Le nom ou le mot de passe ne correspond à aucun compte.</span></div>
				<Input label="Nom" value="Camille" size="lg" id="demo-login-name" />
				<PasswordField label="Mot de passe" value="motdepasse" id="demo-login-pw" />
				<Button variant="primary" size="lg">Se connecter</Button>
			</div>
			<div class="m6-col">
				<p class="micro m5b-cap">Menu utilisateur (compte connecté)</p>
				<div class="row-inline"><UserMenu /><UserMenu compact /></div>
				<p class="micro m5b-cap">Badges sur les affiches</p>
				<div class="row-inline">
					<WatchBadge played />
					<WatchBadge unplayed={3} />
					<WatchBadge unplayed={128} />
					<Tag variant="accent">Cet appareil</Tag>
					<Tag variant="accent" icon="shield">Administrateur</Tag>
					<Tag>Membre</Tag>
				</div>
				<div class="m6-progress"><ProgressLine value={0.42} variant="inline" /><span class="micro tabular">42:10 sur 1:42:00 · reste 1 h 00</span></div>
				<p class="micro m5b-cap">Robustesse du mot de passe</p>
				<div class="m6-strength">
					<PasswordStrength password="abc" />
					<PasswordStrength password="soleil2024" />
					<PasswordStrength password="Soleil-de-nuit" />
					<PasswordStrength password="nuit-étoilée-42!" />
				</div>
				<div class="m6-switches panel">
					<Switch label="Partagée" hint="Visible par tous les utilisateurs du serveur." checked />
					<Switch label="Masquer les contenus non classés" />
				</div>
			</div>
		</div>
		<div class="m6-cards">
			<PosterCard title={films[2].title} year={films[2].year} color={dominant(films[2].hue, films[2].hue2)} href="/design" progress={0.42}>
				{#snippet art()}{@const f = films[2]}<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />{/snippet}
			</PosterCard>
			<PosterCard title={films[3].title} year={films[3].year} color={dominant(films[3].hue, films[3].hue2)} href="/design" played>
				{#snippet art()}{@const f = films[3]}<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />{/snippet}
			</PosterCard>
			<PosterCard title={films[4].title} subtitle="2 saisons" color={dominant(films[4].hue, films[4].hue2)} href="/design" unplayed={3}>
				{#snippet art()}{@const f = films[4]}<PosterArt title={f.title} year={f.year} credit={f.director} hue={f.hue} hue2={f.hue2} sat={f.sat} scene={f.scene} layout={f.layout} />{/snippet}
			</PosterCard>
			<CollectionCard collection={{ id: 'c0', name: 'Soirées d’hiver', kind: 'manual', count: 0, images: {}, shared: true }} posters={[]} />
		</div>
	</section>

	<!-- M5c : agents appairés, sélecteurs de dossiers -->
	<section class="block" aria-labelledby="s-m5c">
		<div class="heading">
			<span class="num tabular">14</span>
			<h2 id="s-m5c">Agents et dossiers (M5c)</h2>
			<p>Parcours guidé pour ajouter une machine sans agent.toml : agent, code d’appairage, dossier, vérification. Deux composants génériques : l’indicateur d’étapes et l’explorateur de dossiers (dossiers seulement, fil d’Ariane cliquable).</p>
		</div>
		<div class="m5b-panel panel">
			<p class="micro m5b-cap">Stepper : étapes terminées, en cours, à venir (« Code » sautée pour un agent déjà appairé)</p>
			<Stepper
				steps={[{ label: 'Agent', onclick: () => (stepDemo = 0) }, { label: 'Code', skipped: stepDemo > 1 }, { label: 'Dossier', onclick: () => (stepDemo = 2) }, { label: 'Vérification' }]}
				current={stepDemo}
				label="Exemple d’étapes"
			/>
			<div class="row-inline">
				{#each [0, 1, 2, 3] as i (i)}<Button variant={stepDemo === i ? 'primary' : 'tertiary'} onclick={() => (stepDemo = i)}>Étape {i + 1}</Button>{/each}
			</div>
		</div>
		<div class="m5b-grid">
			<div class="m5b-panel panel">
				<p class="micro m5b-cap">FolderPicker (« backups » simule un dossier refusé)</p>
				<FolderPicker load={demoLoad} bind:value={demoFolder} label="Exemple d’explorateur" errorText={() => 'Dossier hors des emplacements autorisés sur cette machine.'}>
					{#snippet footer(path)}
						<p class="micro m5b-note">{path ? `Choisi : ${path}` : 'Ouvrez un emplacement.'}</p>
						<Button variant="primary" icon="folder" disabled={!path} onclick={() => toasts.show('Dossier partagé', { detail: path })}>Partager ce dossier</Button>
					{/snippet}
				</FolderPicker>
			</div>
			<AgentHelp open />
		</div>
	</section>

	<!-- M6b : synchronisation Trakt / Simkl, Ma liste -->
	<section class="block" aria-labelledby="s-m6b">
		<div class="heading">
			<span class="num tabular">15</span>
			<h2 id="s-m6b">Synchronisation et Ma liste (M6b)</h2>
			<p>
				Pastille de service (initiale sur tuile sobre, jamais le logo ni la couleur de la marque), code d’appairage copiable, panneau d’appairage avec
				compte à rebours, journal compact (conflits avec le côté retenu, imports, exports, erreurs), onglets de section et bouton « Ma liste ».
			</p>
		</div>
		<div class="m5b-grid">
			<div class="m5b-panel panel">
				<p class="micro m5b-cap">ServiceMark · CopyCode</p>
				<div class="row-inline"><ServiceMark name="Trakt" /><ServiceMark name="Simkl" /><ServiceMark name="Simkl" muted /></div>
				<CopyCode code="A1B2C3D4" label="Code d’exemple" />
				<p class="micro m5b-cap">SubNav (onglets de section)</p>
				<SubNav items={[{ href: '/design', label: 'Compte', icon: 'user' }, { href: '/design/sync', label: 'Synchronisation', icon: 'refresh' }]} label="Exemple d’onglets" exact={['/design']} />
				<p class="micro m5b-cap">Ma liste (fiche d’un film)</p>
				<div class="row-inline">
					<Button size="lg" icon="bookmark">Ma liste</Button>
					<Button size="lg" icon="bookmark" iconFilled class="m6b-on">Dans ma liste</Button>
				</div>
			</div>
			<div class="m5b-panel panel">
				<p class="micro m5b-cap">DeviceCodePanel (en attente)</p>
				<DeviceCodePanel code={demoCode} service="Trakt" phase="waiting" now={syncNow} lifetime={10 * 60_000} oncancel={() => toasts.show('Appairage annulé')} onrestart={() => {}} />
				<p class="micro m5b-cap">DeviceCodePanel (expiré)</p>
				<DeviceCodePanel code={demoExpired} service="Simkl" phase="expired" now={syncNow} oncancel={() => {}} onrestart={() => toasts.show('Nouveau code demandé')} />
			</div>
		</div>
		<div class="m5b-panel panel">
			<p class="micro m5b-cap">SyncJournal</p>
			<SyncJournal events={demoEvents} service="trakt" now={syncNow} step={4} />
		</div>
	</section>

	<!-- Notifications -->
	<section class="block" aria-labelledby="s-toasts">
		<div class="heading">
			<span class="num tabular">10</span>
			<h2 id="s-toasts">Notifications</h2>
			<p>En bas à droite (au-dessus de la barre d’onglets sur mobile), sur surface relevée.</p>
		</div>
		<div class="row-inline">
			<Button variant="secondary" onclick={() => toasts.show('Bibliothèque « Films » analysée', { detail: '12 nouveaux titres' })}>Notification simple</Button>
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
	.m6-grid {
		display: grid;
		grid-template-columns: minmax(0, 400px) minmax(0, 1fr);
		gap: var(--sp-6);
		align-items: start;
	}

	.m6-cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 160px));
		gap: var(--gap);
		margin-top: var(--sp-5);
	}

	.m6-login {
		display: grid;
		gap: var(--sp-4);
		padding: var(--sp-6);
	}

	.m6-title {
		font-size: 1.5rem;
		font-weight: var(--fw-heavy);
	}

	.m6-alert {
		display: flex;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--danger) 12%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 30%, transparent);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.m6-alert :global(svg) {
		color: var(--danger);
		flex: none;
	}

	.m6-alert strong {
		color: var(--text);
	}

	.m6-col {
		display: grid;
		gap: var(--sp-3);
		min-width: 0;
	}

	.m6-progress {
		display: grid;
		gap: var(--sp-2);
		max-width: 360px;
		color: var(--text-muted);
	}

	.m6-strength {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-4);
		max-width: 560px;
	}

	.m6-switches {
		display: grid;
		gap: var(--sp-4);
		max-width: 560px;
		padding: var(--sp-4) var(--sp-5);
	}

	@media (max-width: 899px) {
		.m6-grid,
		.m6-strength {
			grid-template-columns: minmax(0, 1fr);
		}
	}

	.m5b-sources {
		display: grid;
		gap: var(--sp-2);
		max-width: 820px;
	}

	.m5b-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--gap);
		align-items: start;
	}

	.m5b-panel {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-5);
	}

	.m5b-cap,
	.m5b-note {
		color: var(--text-muted);
	}

	.m5b-cards {
		display: grid;
		grid-template-columns: repeat(2, 160px) auto;
		align-items: center;
		gap: var(--gap);
	}

	@media (max-width: 767px) {
		.m5b-grid {
			grid-template-columns: minmax(0, 1fr);
		}

		.m5b-cards {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}

	.m4-art {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: var(--gap);
		margin-bottom: var(--sp-5);
	}

	.m4-art figure {
		display: grid;
		gap: var(--sp-2);
		width: 150px;
		color: var(--text-muted);
	}

	.m4-art figure.wide {
		width: 280px;
	}

	.m4-frame {
		border-radius: var(--r-lg);
		overflow: hidden;
		box-shadow: var(--shadow-1), inset 0 0 0 1px var(--line);
	}

	.m4-frame.round {
		border-radius: 50%;
	}

	.m4-wide {
		position: relative;
		aspect-ratio: 16 / 9;
	}

	.m4-dialog-text {
		color: var(--text-muted);
	}

	.design {
		display: grid;
		gap: var(--sp-8);
	}

	.intro {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-6);
		padding: var(--sp-7) var(--sp-7);
	}

	.intro-copy {
		max-width: 720px;
	}

	.intro-mark {
		flex: none;
		display: grid;
		place-items: center;
		width: 168px;
		height: 168px;
		border-radius: 36px;
		background: var(--bg);
		box-shadow: var(--shadow-2), inset 0 0 0 1px var(--line);
	}

	.eyebrow {
		color: var(--accent-ink);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		margin-bottom: var(--sp-3);
	}

	.lede {
		margin-top: var(--sp-4);
		max-width: 62ch;
		color: var(--text-muted);
		font-size: 1rem;
	}

	.block {
		display: grid;
		gap: var(--sp-5);
		min-width: 0;
	}

	.heading {
		display: grid;
		grid-template-columns: auto 1fr;
		column-gap: var(--sp-4);
		align-items: baseline;
	}

	.heading h2 {
		font-size: 1.75rem;
		line-height: 1.25;
	}

	.heading p {
		grid-column: 2;
		color: var(--text-muted);
		max-width: 70ch;
		margin-top: var(--sp-1);
	}

	.num {
		display: inline-grid;
		place-items: center;
		min-width: 36px;
		height: 28px;
		padding: 0 8px;
		border-radius: var(--r-full);
		background: var(--accent-soft);
		color: var(--accent-ink);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		transform: translateY(-3px);
	}

	:global(.btn.secondary.m6b-on) {
		color: var(--accent-ink);
	}

	:global(.btn.secondary.m6b-on svg) {
		color: var(--accent);
	}

	.row-inline {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
	}

	.sw-name {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.sw-value,
	.sw-role {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	.card {
		min-width: 0;
		padding: var(--sp-5);
	}

	.card h3 {
		font-size: var(--fs-h3);
		color: var(--text);
		margin-bottom: var(--sp-4);
	}

	.card h3.mt {
		margin-top: var(--sp-6);
	}

	/* Themes */
	.themes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 172px), 1fr));
		gap: var(--gap);
	}

	.theme-card {
		display: block;
		width: 100%;
		text-align: left;
		border-radius: var(--r-lg);
		overflow: hidden;
		background: var(--surface);
		box-shadow: var(--shadow-1), 0 0 0 1px var(--line);
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
		box-shadow: var(--shadow-2), 0 0 0 2px var(--accent);
	}

	.tp {
		display: flex;
		gap: 8px;
		height: 128px;
		padding: 10px;
		background: var(--bg);
	}

	.tp-side {
		display: grid;
		align-content: start;
		gap: 5px;
		width: 44px;
		flex: none;
	}

	.tp-logo {
		display: block;
		height: 18px;
		padding-left: 2px;
	}

	.tp-nav {
		height: 10px;
		border-radius: 3px;
		background: var(--surface);
	}

	.tp-nav.on {
		background: var(--surface-active);
		box-shadow: inset 3px 0 0 var(--accent);
	}

	.tp-main {
		flex: 1;
		display: grid;
		grid-template-rows: 1fr auto;
		gap: 6px;
		min-width: 0;
	}

	.tp-hero {
		position: relative;
		overflow: hidden;
		border-radius: 8px;
		isolation: isolate;
	}

	.tp-hero::after {
		content: '';
		position: absolute;
		inset: 0;
		background: var(--scrim-side);
	}

	.tp-hero-copy {
		position: absolute;
		left: 8px;
		bottom: 7px;
		z-index: 1;
		display: grid;
		justify-items: start;
		gap: 4px;
	}

	.tp-title {
		font-size: 0.6875rem;
		font-weight: var(--fw-heavy);
		color: var(--on-image);
	}

	.tp-btn {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		height: 14px;
		padding: 0 6px;
		border-radius: var(--r-full);
		background: var(--accent);
		color: var(--accent-contrast);
		font-size: 0.5rem;
		font-weight: var(--fw-bold);
	}

	.tp-panel {
		display: grid;
		gap: 4px;
		padding: 6px 8px;
		border-radius: 6px;
		background: var(--tint), var(--surface);
	}

	.tp-line {
		height: 4px;
		border-radius: 2px;
		background: var(--text);
	}

	.tp-line.muted {
		background: var(--text-muted);
	}

	.w60 {
		width: 60%;
	}

	.w40 {
		width: 40%;
	}

	.tp-bar {
		height: 3px;
		border-radius: 2px;
		background: var(--line-strong);
	}

	.tp-bar span {
		display: block;
		width: 58%;
		height: 100%;
		border-radius: inherit;
		background: var(--accent);
	}

	.theme-foot {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4) 14px;
		color: var(--text);
	}

	.theme-name,
	.theme-mood {
		display: block;
	}

	.theme-name {
		font-size: var(--fs-h3);
		font-weight: var(--fw-bold);
		line-height: 1.3;
	}

	.theme-mood {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.theme-state {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		flex: none;
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.theme-state.on {
		color: var(--accent-ink);
	}

	/* Colours */
	.swatches {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.swatch {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.chip-color {
		height: 72px;
		margin-bottom: var(--sp-2);
		border-radius: var(--r-md);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	/* Type */
	.type-scale {
		display: grid;
		padding: var(--sp-2) var(--sp-5);
	}

	.type-row {
		display: grid;
		grid-template-columns: 240px minmax(0, 1fr);
		gap: var(--sp-5);
		align-items: baseline;
		padding: var(--sp-4) 0;
		border-bottom: 1px solid var(--line);
	}

	.type-row:last-child {
		border-bottom: 0;
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
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: var(--fw-heavy);
		letter-spacing: -0.02em;
	}

	.specimen.h1 {
		font-size: var(--fs-h1);
		line-height: var(--lh-h1);
		font-weight: var(--fw-bold);
		letter-spacing: -0.015em;
	}

	.specimen.h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
		font-weight: var(--fw-bold);
	}

	.specimen.h3 {
		font-size: var(--fs-h3);
		line-height: var(--lh-h3);
		font-weight: var(--fw-bold);
	}

	.specimen.body {
		max-width: 70ch;
	}

	.specimen.small {
		font-size: var(--fs-small);
		line-height: var(--lh-small);
		color: var(--text-muted);
	}

	.specimen.micro {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	/* Foundations */
	.foundations {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 360px), 1fr));
		gap: var(--gap);
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
		border-radius: var(--r-full);
		background: var(--accent);
	}

	.radii,
	.shadows {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-5);
	}

	.radii li,
	.shadows li {
		display: grid;
		justify-items: start;
		gap: 4px;
	}

	.radius-box {
		width: 64px;
		height: 64px;
		margin-bottom: 4px;
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.shadow-box {
		width: 88px;
		height: 64px;
		margin-bottom: 4px;
		border-radius: var(--r-lg);
		background: var(--surface-raised);
	}

	.shadow-box.tinted {
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	/* Motion */
	.motion {
		display: grid;
		gap: var(--sp-2);
	}

	.motion-row {
		display: grid;
		grid-template-columns: 120px 200px minmax(0, 1fr) minmax(0, 1.2fr);
		align-items: center;
		gap: var(--sp-4);
		padding: var(--sp-3) 0;
		border-bottom: 1px solid var(--line);
	}

	.track {
		position: relative;
		height: 3px;
		border-radius: var(--r-full);
		background: var(--line-strong);
	}

	.puck {
		position: absolute;
		top: -5px;
		left: 0;
		width: 13px;
		height: 13px;
		border-radius: 50%;
		background: var(--accent);
		transition-property: left;
		transition-timing-function: var(--ease);
	}

	.puck.played {
		left: calc(100% - 13px);
	}

	/* Brand & icons */
	.brand {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-5);
	}

	.brand :global(.word) {
		font-size: 2.5rem;
	}

	.brand-sizes {
		display: flex;
		align-items: flex-end;
		gap: var(--sp-3);
	}

	.app-icon {
		display: grid;
		place-items: center;
		background: var(--bg);
		box-shadow: var(--shadow-1), inset 0 0 0 1px var(--line-strong);
	}

	.app-icon.lg {
		width: 88px;
		height: 88px;
		border-radius: 22px;
	}

	.app-icon.md {
		width: 60px;
		height: 60px;
		border-radius: 16px;
	}

	.app-icon.sm {
		width: 38px;
		height: 38px;
		border-radius: 10px;
	}

	.tool-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.icons {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(104px, 1fr));
		gap: 8px;
	}

	.icons li {
		display: grid;
		justify-items: center;
		gap: var(--sp-2);
		padding: var(--sp-4) var(--sp-2) var(--sp-3);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
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
		grid-template-columns: 110px repeat(4, minmax(0, max-content));
		gap: var(--sp-4) var(--sp-6);
		align-items: center;
		overflow-x: auto;
		padding: var(--sp-5);
	}

	.col-head,
	.row-head {
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	/* Chips & fields */
	.chips {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-2);
	}

	.states-row {
		margin-top: var(--sp-4);
		padding-top: var(--sp-4);
		border-top: 1px solid var(--line);
	}

	.tag-on-image {
		position: relative;
		display: inline-flex;
		gap: 6px;
		padding: 10px;
		border-radius: var(--r-sm);
		overflow: hidden;
		isolation: isolate;
	}

	.tag-on-image :global(.art) {
		z-index: -1;
	}

	.fields {
		display: grid;
		gap: var(--sp-4);
		align-content: start;
	}

	.fields h3 {
		margin-bottom: 0;
	}

	/* Cards */
	.posters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(46%, 150px), 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.continue-demo {
		display: grid;
		grid-template-columns: 1.4fr 1fr 1fr;
		grid-auto-rows: 200px;
		gap: var(--gap);
	}

	.thumbs {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 1fr));
		gap: var(--sp-5) var(--gap);
	}

	.cats {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(46%, 170px), 1fr));
		grid-auto-rows: 84px;
		gap: 10px;
	}

	.loading {
		display: grid;
		gap: var(--sp-3);
		align-content: start;
	}

	@media (max-width: 767px) {
		.design {
			gap: var(--sp-7);
		}

		.intro {
			flex-direction: column-reverse;
			align-items: flex-start;
			padding: var(--sp-5);
		}

		.intro-mark {
			width: 96px;
			height: 96px;
			border-radius: 24px;
		}

		.intro-mark :global(svg) {
			width: 52px;
			height: 52px;
		}

		.heading {
			grid-template-columns: 1fr;
			justify-items: start;
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

		.matrix {
			grid-template-columns: 90px repeat(4, max-content);
		}

		.continue-demo {
			grid-template-columns: 1fr 1fr;
			grid-auto-rows: 132px;
		}

		.ct-featured {
			grid-column: 1 / -1;
			grid-row: span 2;
		}
	}
</style>
