<script lang="ts">
	// Administration: application credentials of one synchronisation service (GET/PUT/DELETE /sync/apps).
	// The client ID comes back masked and the secret never comes back: the form only replaces them.
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import type { SyncApp, SyncService } from '$lib/api/types';
	import { OOB_REDIRECT, SERVICE_INFO } from '$lib/sync';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import ConfirmDialog from '../ConfirmDialog.svelte';
	import Icon from '../Icon.svelte';
	import Tag from '../Tag.svelte';
	import SecretField from '../sources/SecretField.svelte';
	import ServiceMark from './ServiceMark.svelte';

	interface Props {
		service: SyncService;
		app: SyncApp;
		onchange: (app: SyncApp) => void;
	}

	let { service, app, onchange }: Props = $props();

	const info = $derived(SERVICE_INFO[service]);
	const needsSecret = $derived(service === 'trakt');
	const uid = $props.id();

	let editing = $state(false);
	let clientId = $state('');
	let clientSecret = $state('');
	let saving = $state(false);
	let formError = $state('');
	let deleteOpen = $state(false);
	let deleting = $state(false);
	let copied = $state(false);

	const showForm = $derived(app.source !== 'config' && (!app.configured || editing));

	function startEdit() {
		clientId = '';
		clientSecret = '';
		formError = '';
		editing = true;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		formError = '';
		const id = clientId.trim();
		const secret = clientSecret.trim();
		if (!id) return (formError = 'Collez le Client ID de l’application.');
		if (needsSecret && !secret) return (formError = 'Trakt demande aussi le Client Secret.');
		saving = true;
		try {
			const got = await api.setSyncApp(service, needsSecret ? { clientId: id, clientSecret: secret } : { clientId: id });
			onchange(got ?? { configured: true, source: 'database' });
			clientId = clientSecret = '';
			editing = false;
			toasts.show(`Application ${info.name} enregistrée`, { detail: 'Chaque utilisateur peut maintenant relier son compte depuis son profil.' });
		} catch (err) {
			if (err instanceof ApiError && err.code === 'managed_by_config') onchange({ ...app, configured: true, source: 'config' });
			formError = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	async function remove() {
		deleting = true;
		try {
			await api.deleteSyncApp(service);
			deleteOpen = false;
			onchange({ configured: false, source: 'none' });
			toasts.show(`Application ${info.name} supprimée`, { detail: 'Les comptes reliés devront être reconnectés après une nouvelle configuration.' });
		} catch (err) {
			toasts.show('Suppression impossible', { detail: errorMessage(err) });
		} finally {
			deleting = false;
		}
	}

	async function copyRedirect() {
		try {
			await navigator.clipboard.writeText(OOB_REDIRECT);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			/* the value is selectable */
		}
	}

	const createUrl = $derived(service === 'trakt' ? 'https://trakt.tv/oauth/applications' : 'https://simkl.com/settings/developer/');
	const createLabel = $derived(service === 'trakt' ? 'trakt.tv/oauth/applications' : 'simkl.com/settings/developer');
</script>

<article class="app panel" aria-labelledby="app-{uid}">
	<header class="head">
		<ServiceMark name={info.name} size={40} muted={!app.configured} />
		<div class="title">
			<h2 id="app-{uid}">{info.name}</h2>
			<p class="site">{info.site}</p>
		</div>
		{#if app.source === 'config'}
			<Tag variant="accent" icon="lock">Configuration du serveur</Tag>
		{:else if app.configured}
			<Tag variant="accent" icon="check">Configuré</Tag>
		{:else}
			<Tag>Non configuré</Tag>
		{/if}
	</header>

	{#if app.configured}
		<dl class="facts">
			<div>
				<dt>Client ID</dt>
				<dd class="mono">{app.clientId ?? 'masqué'}</dd>
			</div>
			{#if needsSecret}
				<div>
					<dt>Client Secret</dt>
					<dd>enregistré, jamais réaffiché</dd>
				</div>
			{/if}
			<div>
				<dt>Origine</dt>
				<dd>{app.source === 'config' ? 'Fichier de configuration ou variables d’environnement' : 'Enregistré depuis cette page'}</dd>
			</div>
		</dl>
	{/if}

	{#if app.source === 'config'}
		<p class="note">
			<Icon name="info" size={16} />
			<span>
				Défini par la configuration du serveur
				(<code>{service === 'trakt' ? 'AETHER_TRAKT_CLIENT_ID' : 'AETHER_SIMKL_CLIENT_ID'}</code> ou section <code>[sync]</code> du TOML) : modifiez-la puis redémarrez Aether pour changer ces identifiants.
			</span>
		</p>
	{:else if showForm}
		<details class="guide" open={!app.configured}>
			<summary><Icon name="info" size={16} />Créer l’application sur {info.site}<span class="chev" aria-hidden="true"><Icon name="chevron-down" size={16} /></span></summary>
			<ol>
				{#if service === 'trakt'}
					<li>
						Connectez-vous sur trakt.tv puis ouvrez
						<a href={createUrl} target="_blank" rel="noopener noreferrer">{createLabel}<Icon name="external" size={13} /><span class="visually-hidden"> (nouvel onglet)</span></a>
						et choisissez « New application ».
					</li>
					<li><strong>Name</strong> : « Aether » ou le nom de votre serveur (vos utilisateurs le verront au moment d’autoriser l’accès).</li>
					<li>
						<strong>Redirect uri</strong> : collez exactement
						<span class="redirect"><code>{OOB_REDIRECT}</code><button type="button" onclick={copyRedirect}><Icon name={copied ? 'check' : 'copy'} size={14} />{copied ? 'Copié' : 'Copier'}</button></span>
						L’appairage se fait par code : aucune redirection vers ce serveur, qui n’a pas besoin d’être accessible depuis Internet.
					</li>
					<li>Laissez <strong>Javascript (cors) origins</strong> vide ; cochez <strong>/scrobble</strong> si la case est proposée.</li>
					<li>Enregistrez (« Save app »), puis recopiez ci-dessous le <strong>Client ID</strong> et le <strong>Client Secret</strong> affichés.</li>
				{:else}
					<li>
						Connectez-vous sur simkl.com puis ouvrez
						<a href={createUrl} target="_blank" rel="noopener noreferrer">{createLabel}<Icon name="external" size={13} /><span class="visually-hidden"> (nouvel onglet)</span></a>
						(« Create new app »).
					</li>
					<li><strong>Name</strong> : « Aether » ou le nom de votre serveur.</li>
					<li>
						<strong>Redirect URI</strong> : indiquez
						<span class="redirect"><code>{OOB_REDIRECT}</code><button type="button" onclick={copyRedirect}><Icon name={copied ? 'check' : 'copy'} size={14} />{copied ? 'Copié' : 'Copier'}</button></span>
						Aether utilise l’appairage par code PIN : la redirection n’est jamais utilisée.
					</li>
					<li>
						Choisissez l’authentification <strong>AUTH V1 / PIN</strong> : un Client ID « AUTH V2 » est refusé par l’appairage par code (le message d’erreur de Simkl s’affiche alors tel quel).
					</li>
					<li>Enregistrez, puis recopiez ci-dessous le <strong>Client ID</strong>. Simkl n’a pas besoin du secret pour l’appairage par PIN.</li>
				{/if}
			</ol>
			{#if service === 'simkl'}
				<p class="later micro">
					Simkl prévoit de retirer AUTH V1 vers avril 2027 ; Aether suivra la migration.
					Documentation : <a href="https://api.simkl.org" target="_blank" rel="noopener noreferrer">api.simkl.org<span class="visually-hidden"> (nouvel onglet)</span></a>.
				</p>
			{:else}
				<p class="later micro">
					Documentation : <a href="https://developer.trakt.tv" target="_blank" rel="noopener noreferrer">developer.trakt.tv<span class="visually-hidden"> (nouvel onglet)</span></a>.
				</p>
			{/if}
		</details>

		<form class="form" onsubmit={save} novalidate>
			<div class="f-field">
				<label class="f-label" for="cid-{uid}">Client ID</label>
				<input
					id="cid-{uid}"
					class="f-input f-mono"
					bind:value={clientId}
					autocomplete="off"
					spellcheck="false"
					autocapitalize="off"
					placeholder={app.configured ? 'Nouveau Client ID' : 'Collez le Client ID'}
					aria-invalid={!!formError && !clientId.trim()}
				/>
			</div>
			{#if needsSecret}
				<SecretField id="cs-{uid}" label="Client Secret" bind:value={clientSecret} placeholder="Collez le Client Secret" hint="Chiffré sur le serveur ; il ne sera plus jamais affiché." />
			{/if}
			{#if formError}<p class="f-err" role="alert"><Icon name="alert" size={14} />{formError}</p>{/if}
			<div class="row-end">
				{#if editing}<Button variant="tertiary" onclick={() => (editing = false)}>Annuler</Button>{/if}
				<Button type="submit" variant="primary" disabled={saving} aria-busy={saving}>{app.configured ? 'Remplacer' : 'Enregistrer'}</Button>
			</div>
		</form>
	{:else}
		<div class="row-end">
			<Button variant="tertiary" icon="trash" onclick={() => (deleteOpen = true)}>Supprimer</Button>
			<Button icon="key" onclick={startEdit}>Remplacer les identifiants</Button>
		</div>
	{/if}
</article>

<ConfirmDialog bind:open={deleteOpen} title="Supprimer l’application {info.name} ?" confirmLabel="Supprimer" danger busy={deleting} onconfirm={remove}>
	<p>Les utilisateurs ne pourront plus relier de compte {info.name}, et les connexions existantes passeront en erreur à leur prochaine synchronisation.</p>
	<p>Les données dans Aether (vus, notes, Ma liste) ne sont pas touchées.</p>
</ConfirmDialog>

<style>
	.app {
		display: grid;
		gap: var(--sp-5);
		padding: var(--sp-5) var(--sp-6) var(--sp-6);
		min-width: 0;
	}

	.head {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-3) var(--sp-4);
	}

	.title {
		display: grid;
		flex: 1;
		min-width: 0;
	}

	.title h2 {
		font-size: var(--fs-h2);
		line-height: 1.2;
	}

	.site {
		color: var(--text-faint);
		font-size: var(--fs-micro);
	}

	.facts {
		display: grid;
		gap: var(--sp-3);
	}

	.facts div {
		display: grid;
		grid-template-columns: 140px minmax(0, 1fr);
		gap: var(--sp-3);
		align-items: baseline;
	}

	dt {
		font-size: var(--fs-micro);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--fs-small);
		overflow-wrap: anywhere;
	}

	.mono {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		color: var(--text);
	}

	.note {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--bg) 45%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.note :global(svg) {
		flex: none;
		margin-top: 2px;
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125em;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.guide {
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	summary {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		min-height: 44px;
		padding: 0 var(--sp-4);
		border-radius: inherit;
		font-weight: var(--fw-bold);
		color: var(--text);
		cursor: pointer;
		list-style: none;
	}

	summary::-webkit-details-marker {
		display: none;
	}

	.chev {
		display: inline-flex;
		margin-left: auto;
		transition: transform var(--dur-fast) var(--ease);
	}

	.guide[open] .chev {
		transform: rotate(180deg);
	}

	ol {
		display: grid;
		gap: var(--sp-3);
		margin: 0;
		padding: 0 var(--sp-5) var(--sp-4) calc(var(--sp-5) + 18px);
	}

	.later {
		padding: 0 var(--sp-5) var(--sp-4);
		color: var(--text-faint);
	}

	.later a {
		color: var(--accent-ink);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	li strong {
		color: var(--text);
	}

	ol a {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--accent-ink);
		font-weight: var(--fw-bold);
		text-decoration: underline;
		text-decoration-color: color-mix(in srgb, currentColor 40%, transparent);
		text-underline-offset: 3px;
	}

	.redirect {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: var(--sp-2);
		margin: var(--sp-2) 0;
		padding: 6px 6px 6px var(--sp-3);
		border-radius: var(--r-sm);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.redirect code {
		user-select: all;
	}

	.redirect button {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		height: 28px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-full);
		background: var(--surface-raised);
		color: var(--text);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
	}

	.redirect button :global(svg) {
		color: var(--accent);
	}

	.form {
		display: grid;
		gap: var(--sp-4);
	}

	.row-end {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	@media (max-width: 767px) {
		.app {
			padding: var(--sp-5) var(--sp-4);
		}

		.facts div {
			grid-template-columns: minmax(0, 1fr);
			gap: 2px;
		}

		.row-end :global(.btn) {
			flex: 1;
		}
	}
</style>
