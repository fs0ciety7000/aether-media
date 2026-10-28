<script lang="ts">
	// First-run assistant (T-080): administrator account, then metadata language and country,
	// TMDB key, first library and a summary. Every step after the account can be skipped; the
	// progress is stored by the server (settings.wizard), so a reload or another device resumes it.
	import { onMount, tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, errorMessage } from '$lib/api/client';
	import { libraries, scans } from '$lib/api/stores.svelte';
	import type { Library, ServerSettings, WizardStep } from '$lib/api/types';
	import { countryLabel, languageLabel, regionOf, WIZARD_STEPS } from '$lib/admin';
	import AuthShell from '$lib/components/auth/AuthShell.svelte';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import PasswordStrength from '$lib/components/auth/PasswordStrength.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon, { type IconName } from '$lib/components/Icon.svelte';
	import Input from '$lib/components/Input.svelte';
	import LocaleFields from '$lib/components/admin/LocaleFields.svelte';
	import TmdbKeyField from '$lib/components/admin/TmdbKeyField.svelte';
	import AddLibraryDialog from '$lib/components/settings/AddLibraryDialog.svelte';
	import { libraryKindInfo } from '$lib/media';
	import { plural } from '$lib/format';
	import { session } from '$lib/session.svelte';
	import { toasts } from '$lib/toast.svelte';

	type Step = 'loading' | Exclude<WizardStep, 'done'>;
	let step = $state<Step>('loading');
	let settings = $state<ServerSettings | null>(null);
	let skipped = $state<WizardStep[]>([]);
	let loadError = $state('');

	// Account
	let name = $state('');
	let password = $state('');
	let confirm = $state('');
	let busy = $state(false);
	let error = $state('');
	let touched = $state(false);
	const mismatch = $derived(touched && !!confirm && confirm !== password);
	const tooShort = $derived(touched && password.length > 0 && password.length < 8);

	// Metadata
	let language = $state('fr-FR');
	let country = $state('');

	// Library
	let addOpen = $state(false);

	const order: WizardStep[] = ['account', 'metadata', 'tmdb', 'library', 'summary'];
	const current = $derived(step === 'loading' ? 0 : order.indexOf(step));

	onMount(async () => {
		await session.init();
		if (session.status === 'setup') {
			step = 'account';
			return;
		}
		if (session.status !== 'authenticated') return void goto('/connexion', { replaceState: true });
		if (!session.isAdmin) return void goto('/', { replaceState: true });
		await loadSettings();
		if (!settings) return;
		if (settings.wizard.completed) {
			session.wizardPending = false;
			return void goto('/', { replaceState: true });
		}
		const s = settings.wizard.step;
		show(s === 'account' || s === 'done' ? 'metadata' : s);
	});

	async function loadSettings() {
		try {
			settings = await api.settings();
			skipped = settings.wizard.skipped ?? [];
			language = settings.metadataLanguage.value || 'fr-FR';
			country = settings.metadataCountry.value ?? '';
			loadError = '';
			libraries.ensure();
			scans.ensure();
		} catch (err) {
			loadError = errorMessage(err);
		}
	}

	async function show(next: Step) {
		step = next;
		await tick();
		document.getElementById('step-title')?.focus();
	}

	/** Moves to `next`, recording it on the server (and whether the step we leave was skipped). */
	async function go(next: Exclude<WizardStep, 'account' | 'done'>, skip?: WizardStep) {
		const from = step as WizardStep;
		const list = skip ? [...new Set([...skipped, skip])] : skipped.filter((s) => s !== from);
		try {
			settings = await api.patchSettings({ wizard: { step: next, skipped: list } });
			skipped = settings.wizard.skipped ?? [];
		} catch (err) {
			toasts.show('Progression non enregistrée', { detail: errorMessage(err) });
			skipped = list;
		}
		show(next);
	}

	async function submitAccount(e: SubmitEvent) {
		e.preventDefault();
		touched = true;
		error = '';
		if (!name.trim()) return (error = 'Choisissez un nom pour le compte administrateur.');
		if (password.length < 8) return (error = 'Le mot de passe doit faire au moins 8 caractères.');
		if (password !== confirm) return (error = 'Les deux mots de passe ne correspondent pas.');
		busy = true;
		try {
			await session.setup(name.trim(), password);
			password = confirm = '';
			await loadSettings();
			show('metadata');
		} catch (err) {
			error = errorMessage(err);
		} finally {
			busy = false;
		}
	}

	async function saveMetadata(e: SubmitEvent) {
		e.preventDefault();
		if (!settings) return;
		busy = true;
		try {
			const patch: Parameters<typeof api.patchSettings>[0] = {};
			if (!settings.metadataLanguage.readOnly) patch.metadataLanguage = language;
			if (!settings.metadataCountry.readOnly) patch.metadataCountry = country;
			if (Object.keys(patch).length) settings = await api.patchSettings(patch);
			await go('tmdb');
		} catch (err) {
			toasts.show('Réglage non enregistré', { detail: errorMessage(err) });
		} finally {
			busy = false;
		}
	}

	function created(lib: Library) {
		scans.queued(lib.id);
		libraries.refresh();
		toasts.show(`Bibliothèque « ${lib.name} » créée`, { detail: 'L’analyse a commencé : les titres apparaissent au fil de l’eau.' });
	}

	async function finish() {
		busy = true;
		try {
			await api.patchSettings({ wizard: { completed: true } });
			session.wizardPending = false;
			toasts.show('Aether est prêt', { detail: 'Retrouvez tous ces réglages dans Réglages › Serveur.' });
			goto('/', { replaceState: true });
		} catch (err) {
			toasts.show('Impossible de terminer', { detail: errorMessage(err) });
		} finally {
			busy = false;
		}
	}

	const libs = $derived(libraries.list);
	const tmdbOn = $derived(!!settings?.tmdbApiKey.set);
	const effectiveCountry = $derived(settings ? settings.metadataCountry.value || regionOf(settings.metadataLanguage.value) : '');

	interface Recap {
		step: WizardStep;
		icon: IconName;
		label: string;
		value: string;
		ok: boolean;
	}
	const recap = $derived<Recap[]>(
		settings
			? [
					{ step: 'account', icon: 'user', label: 'Administrateur', value: session.user?.name ?? '', ok: true },
					{
						step: 'metadata',
						icon: 'globe',
						label: 'Langue et pays',
						value: `${languageLabel(settings.metadataLanguage.value)} · ${countryLabel(effectiveCountry) || 'selon la langue'}`,
						ok: !skipped.includes('metadata')
					},
					{
						step: 'tmdb',
						icon: 'key',
						label: 'TMDB',
						value: tmdbOn ? (settings.tmdbApiKey.readOnly ? 'Clé définie par la configuration' : 'Clé enregistrée') : 'Non configuré : fichiers NFO et images locales seulement',
						ok: tmdbOn
					},
					{
						step: 'library',
						icon: 'layers',
						label: 'Bibliothèques',
						value: libs.length ? libs.map((l) => l.name).join(', ') : 'Aucune pour l’instant',
						ok: libs.length > 0
					}
				]
			: []
	);
</script>

<svelte:head>
	<title>Bienvenue — Aether</title>
</svelte:head>

<AuthShell wide>
	{#if step !== 'loading'}
		<section class="welcome" aria-labelledby="welcome-title">
			<div class="intro">
				<p class="micro eyebrow">Premier lancement</p>
				<h1 id="welcome-title">Bienvenue dans Aether</h1>
				<p class="lead">Quelques minutes pour préparer votre médiathèque. Tout se modifie ensuite dans les réglages, et chaque étape après le compte peut être passée.</p>
				<ol class="steps" aria-label="Étapes">
					{#each WIZARD_STEPS as s, i (s.id)}
						{@const done = i < current}
						{@const passed = done && skipped.includes(s.id)}
						<li class:done class:active={i === current} aria-current={i === current ? 'step' : undefined}>
							<span class="dot tabular" aria-hidden="true">{#if done && !passed}<Icon name="check" size={14} />{:else if passed}<Icon name="arrow-right" size={13} />{:else}{i + 1}{/if}</span>
							<span>{s.label}</span>
							{#if passed}<span class="passed micro">passée</span>{/if}
							{#if done}<span class="visually-hidden">{passed ? '(passée)' : '(terminée)'}</span>{/if}
						</li>
					{/each}
				</ol>
			</div>

			<div class="card" data-step={step}>
				{#if loadError}
					<p class="error" role="alert"><Icon name="alert" size={16} />{loadError}</p>
					<Button icon="refresh" onclick={loadSettings}>Réessayer</Button>
				{:else if step === 'account'}
					<h2 id="step-title" tabindex="-1">Créer le compte administrateur</h2>
					<p class="muted">Ce compte gère les bibliothèques et les autres utilisateurs. Vous pourrez inviter votre famille ensuite.</p>
					<form class="form" onsubmit={submitAccount} novalidate>
						<Input label="Nom" bind:value={name} size="lg" autocomplete="username" autocapitalize="words" maxlength={64} placeholder="Camille" required />
						<div class="pw">
							<PasswordField label="Mot de passe" bind:value={password} autocomplete="new-password" required aria-invalid={tooShort} aria-describedby="pw-strength" />
							<PasswordStrength password={password} id="pw-strength" />
						</div>
						<PasswordField
							label="Confirmation"
							bind:value={confirm}
							autocomplete="new-password"
							required
							aria-invalid={mismatch}
							hint={mismatch ? 'Les deux mots de passe diffèrent.' : undefined}
							onblur={() => (touched = true)}
						/>
						{#if error}<p class="error" role="alert"><Icon name="alert" size={16} />{error}</p>{/if}
						<Button variant="primary" size="lg" type="submit" disabled={busy} aria-busy={busy} icon="arrow-right">
							{busy ? 'Création…' : 'Créer le compte'}
						</Button>
					</form>
				{:else if !settings}
					<p class="muted" aria-busy="true">Chargement…</p>
				{:else if step === 'metadata'}
					<p class="kicker micro">Étape 2 sur 5</p>
					<h2 id="step-title" tabindex="-1">Langue et pays</h2>
					<p class="muted">Bonjour {session.user?.name}. Dans quelle langue voulez-vous les titres et les résumés ? Le pays sert aux classifications d’âge du contrôle parental et aux dates de sortie.</p>
					<form class="form" onsubmit={saveMetadata} novalidate>
						<LocaleFields bind:language bind:country languageField={settings.metadataLanguage} countryField={settings.metadataCountry} />
						<div class="actions">
							<Button variant="primary" size="lg" type="submit" icon="arrow-right" disabled={busy}>{busy ? 'Enregistrement…' : 'Continuer'}</Button>
							<Button variant="tertiary" size="lg" disabled={busy} onclick={() => go('tmdb', 'metadata')}>Passer</Button>
						</div>
					</form>
				{:else if step === 'tmdb'}
					<p class="kicker micro">Étape 3 sur 5 · facultative</p>
					<h2 id="step-title" tabindex="-1">Clé TMDB</h2>
					<p class="muted">The Movie Database fournit affiches, résumés, distributions et classifications des films et séries. La clé est gratuite pour un usage personnel.</p>
					<TmdbKeyField setting={settings.tmdbApiKey} onsaved={(s) => (settings = s)} helpOpen={!settings.tmdbApiKey.set} idp="wiz-tmdb" />
					<div class="actions">
						<Button variant="tertiary" icon="arrow-left" onclick={() => show('metadata')}>Retour</Button>
						<span class="grow"></span>
						{#if tmdbOn}
							<Button variant="primary" size="lg" icon="arrow-right" onclick={() => go('library')}>Continuer</Button>
						{:else}
							<Button variant="secondary" size="lg" onclick={() => go('library', 'tmdb')}>Passer cette étape</Button>
						{/if}
					</div>
				{:else if step === 'library'}
					<p class="kicker micro">Étape 4 sur 5</p>
					<h2 id="step-title" tabindex="-1">Première bibliothèque</h2>
					<p class="muted">Indiquez où vivent vos fichiers : un dossier de ce serveur, une autre machine avec l’agent Aether, ou une seedbox en SFTP. Aether les analyse et récupère affiches et résumés.</p>
					{#if libs.length}
						<ul class="libs" role="list">
							{#each libs as lib (lib.id)}
								{@const info = libraryKindInfo(lib.kind)}
								{@const scan = scans.byLibrary[lib.id]}
								<li>
									<span class="lib-icon" aria-hidden="true"><Icon name={info.icon} size={18} /></span>
									<span class="lib-text">
										<strong>{lib.name}</strong>
										<span>
											{plural(lib.sources.length, 'source', 'sources')}
											{#if scan?.running}· analyse en cours{#if scan.total} ({scan.processed}/{scan.total}){/if}{:else if scan?.finishedAt}· analyse terminée{/if}
										</span>
									</span>
									<Icon name="check-circle" size={18} />
								</li>
							{/each}
						</ul>
					{:else}
						<ul class="kinds" role="list">
							<li><Icon name="film" size={18} />Films</li>
							<li><Icon name="tv" size={18} />Séries</li>
							<li><Icon name="music" size={18} />Musique</li>
						</ul>
					{/if}
					<div class="actions">
						<Button variant={libs.length ? 'secondary' : 'primary'} size="lg" icon="plus" onclick={() => (addOpen = true)}>
							{libs.length ? 'Ajouter une autre bibliothèque' : 'Ajouter une bibliothèque'}
						</Button>
					</div>
					<div class="actions">
						<Button variant="tertiary" icon="arrow-left" onclick={() => show('tmdb')}>Retour</Button>
						<span class="grow"></span>
						{#if libs.length}
							<Button variant="primary" size="lg" icon="arrow-right" onclick={() => go('summary')}>Continuer</Button>
						{:else}
							<Button variant="secondary" size="lg" onclick={() => go('summary', 'library')}>Plus tard</Button>
						{/if}
					</div>
				{:else if step === 'summary'}
					<p class="kicker micro">Étape 5 sur 5</p>
					<h2 id="step-title" tabindex="-1">Tout est prêt</h2>
					<p class="muted">Vérifiez le récapitulatif. Vous pourrez tout changer dans Réglages.</p>
					<ul class="recap" role="list">
						{#each recap as r (r.step)}
							<li class:todo={!r.ok}>
								<span class="r-icon" aria-hidden="true"><Icon name={r.icon} size={18} /></span>
								<span class="r-text">
									<span class="r-label">{r.label}</span>
									<strong>{r.value}</strong>
								</span>
								{#if r.step !== 'account'}
									<Button variant="tertiary" icon="edit" onclick={() => show(r.step as Step)} aria-label="Modifier : {r.label}">Modifier</Button>
								{/if}
							</li>
						{/each}
					</ul>
					<div class="actions">
						<Button variant="tertiary" icon="arrow-left" onclick={() => show('library')}>Retour</Button>
						<span class="grow"></span>
						<Button variant="primary" size="lg" icon="check" disabled={busy} onclick={finish}>{busy ? 'Un instant…' : 'Terminer'}</Button>
					</div>
				{/if}
			</div>
		</section>
	{/if}
</AuthShell>

{#if settings}
	<AddLibraryDialog bind:open={addOpen} oncreated={created} />
{/if}

<style>
	.welcome {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(0, 520px);
		gap: clamp(var(--sp-6), 6vw, 88px);
		align-items: center;
		width: 100%;
	}

	.eyebrow,
	.kicker {
		color: var(--accent-ink);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	h1 {
		margin-top: var(--sp-2);
		font-size: clamp(2rem, 1.4rem + 2vw, 2.75rem);
		line-height: 1.1;
		font-weight: var(--fw-heavy);
		letter-spacing: -0.025em;
	}

	.lead {
		margin-top: var(--sp-4);
		max-width: 46ch;
		color: var(--text-muted);
		font-size: 0.9375rem;
	}

	.steps {
		display: grid;
		gap: var(--sp-3);
		margin-top: var(--sp-6);
	}

	.steps li {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.steps li.active {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.dot {
		display: grid;
		place-items: center;
		flex: none;
		width: 28px;
		height: 28px;
		border-radius: 50%;
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
	}

	.active .dot {
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: none;
	}

	.done .dot {
		background: var(--accent-soft);
		color: var(--accent-ink);
		box-shadow: none;
	}

	.passed {
		padding: 2px 8px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		color: var(--text-muted);
	}

	.card {
		display: grid;
		gap: var(--sp-3);
		padding: var(--sp-6);
		border-radius: var(--r-xl);
		background: var(--tint), var(--surface);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
	}

	h2:focus {
		outline: none;
	}

	.muted {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.form {
		display: grid;
		gap: var(--sp-4);
		margin-top: var(--sp-3);
	}

	.pw {
		display: grid;
		gap: var(--sp-2);
	}

	[data-step='account'] .form > :global(.btn) {
		width: 100%;
		flex-direction: row-reverse;
	}

	.error {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		color: var(--danger);
		font-size: var(--fs-small);
	}

	.kinds {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		margin: var(--sp-2) 0;
	}

	.kinds li {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		height: 34px;
		padding: 0 14px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-medium);
	}

	.kinds :global(svg) {
		color: var(--accent);
	}

	.libs,
	.recap {
		display: grid;
		gap: var(--sp-2);
		margin: var(--sp-2) 0;
	}

	.libs li,
	.recap li {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.libs li > :global(svg:last-child) {
		color: var(--success);
	}

	.lib-icon,
	.r-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 36px;
		height: 36px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.recap li.todo .r-icon {
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.lib-text,
	.r-text {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: 0;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.lib-text strong,
	.r-text strong {
		color: var(--text);
		font-size: var(--fs-body);
		overflow-wrap: anywhere;
	}

	.r-label {
		font-size: var(--fs-micro);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.recap li.todo strong {
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}

	.grow {
		flex: 1;
	}

	@media (max-width: 899px) {
		.welcome {
			grid-template-columns: minmax(0, 1fr);
			max-width: 560px;
			margin: 0 auto;
		}

		.steps {
			display: flex;
			flex-wrap: wrap;
			gap: var(--sp-2) var(--sp-4);
			margin-top: var(--sp-5);
		}

		.steps li {
			font-size: var(--fs-small);
		}

		.steps li:not(.active) > span:not(.dot):not(.visually-hidden) {
			display: none;
		}
	}

	@media (max-width: 767px) {
		.card {
			padding: var(--sp-5);
		}

		.actions :global(.btn) {
			flex: 1 1 auto;
		}

		.grow {
			display: none;
		}
	}
</style>
