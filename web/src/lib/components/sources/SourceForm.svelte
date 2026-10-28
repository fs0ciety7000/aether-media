<script lang="ts">
	// Fields of one source (local folder, Aether agent or SFTP) + "Tester la connexion" with its inline result.
	// The parent owns the draft and decides when the source can be saved (see isTested in $lib/sources).
	import { onDestroy, tick } from 'svelte';
	import { api, ApiError, errorMessage, isAbort } from '$lib/api/client';
	import type { Source } from '$lib/api/types';
	import {
		draftToInput,
		isTested,
		sourceErrorMessage,
		testKey,
		validateDraft,
		type DraftErrors,
		type SourceDraft
	} from '$lib/sources';
	import { agents } from '$lib/api/stores.svelte';
	import Button from '../Button.svelte';
	import FolderPicker from '../FolderPicker.svelte';
	import Icon from '../Icon.svelte';
	import AgentHelp from './AgentHelp.svelte';
	import AgentWizard from './AgentWizard.svelte';
	import SecretField from './SecretField.svelte';
	import SourceStatusPill from './SourceStatusPill.svelte';

	interface Props {
		draft: SourceDraft;
		errors?: DraftErrors;
		/** Editing an existing source: secrets may stay empty (unchanged). */
		original?: Source;
		/** Prefix for element ids (several forms may coexist in the DOM). */
		idp?: string;
		/** Forces a "testing" look in the design gallery. */
		demoTesting?: boolean;
	}

	let { draft = $bindable(), errors = $bindable({}), original, idp = 'src', demoTesting = false }: Props = $props();

	/** Share name sent before one is chosen (see test()). */
	const editing = $derived(!!original);
	let testing = $state(false);
	let requestError = $state<string | null>(null);
	let ctrl: AbortController | null = null;
	let resultEl = $state<HTMLElement>();

	onDestroy(() => ctrl?.abort());

	const tested = $derived(isTested(draft));
	/** M5c: new agent source → guided pairing; editing a paired one → summary. */
	const paired = $derived(draft.kind === 'agent' && draft.agent.mode === 'paired');
	const guided = $derived(paired && !editing);
	let wizard = $state<AgentWizard>();
	const pairedAgent = $derived(paired ? agents.get(draft.agent.agentId) : undefined);
	$effect(() => {
		if (paired && editing) agents.ensure();
	});

	// Local folder picker (GET /fs/browse).
	let pickerOpen = $state(false);
	let picked = $state('');
	const browseServer = (path: string, signal: AbortSignal) => api.fsBrowse(path, signal);
	function browseError(err: unknown): string {
		const e = err as { status?: number };
		if (e?.status === 403) return 'Dossier hors des emplacements autorisés sur le serveur.';
		if (e?.status === 404) return 'Dossier introuvable ou illisible par le serveur.';
		return errorMessage(err);
	}
	async function usePicked(path: string) {
		if (!path) return;
		draft.root = path;
		clear('root');
		pickerOpen = false;
		await tick();
		document.getElementById(id('root'))?.focus();
	}
	/** Picker starts at the typed folder when it looks absolute. */
	const pickerStart = $derived(draft.kind === 'local' && draft.root.trim().startsWith('/') ? draft.root.trim().replace(/\/+$/, '') || '/' : '');
	const stale = $derived(!!draft.test && draft.test.key !== testKey(draft));
	const missingSecret = $derived.by(() => {
		if (!editing) return false;
		if (draft.kind === 'agent') return draft.agent.mode === 'token' && !draft.agent.token.trim();
		if (draft.kind === 'sftp') return draft.sftp.auth === 'password' ? !draft.sftp.password : !draft.sftp.privateKey.trim();
		return false;
	});
	const fingerprint = $derived(draft.test?.hostKey);
	const errCode = $derived(draft.test && !draft.test.ok ? (draft.test.error ?? '') : '');

	/** Clears a field error as soon as the user edits it. */
	function clear(field: keyof DraftErrors) {
		if (errors[field]) errors = { ...errors, [field]: undefined };
	}

	async function focusFirstError() {
		await tick();
		document.querySelector<HTMLElement>(`[data-source-form="${idp}"] [aria-invalid="true"]`)?.focus();
	}

	export async function test() {
		if (guided) return wizard?.verify();
		if (testing) return;
		const e = validateDraft(draft, { editing, forTest: true });
		// Testing needs the secret even when editing.
		if (missingSecret) {
			if (draft.kind === 'agent') e.token = 'Saisissez le jeton pour tester la connexion.';
			else if (draft.sftp.auth === 'password') e.password = 'Saisissez le mot de passe pour tester la connexion.';
			else e.privateKey = 'Collez la clé privée pour tester la connexion.';
		}
		errors = e;
		if (Object.values(e).some(Boolean)) return focusFirstError();

		ctrl?.abort();
		ctrl = new AbortController();
		testing = true;
		requestError = null;
		const key = testKey(draft);
		try {
			const input = draftToInput(draft);
			// The server requires a share even to list them; an unknown share still answers with the
			// list (ok:false + agent). Before the user has picked one, ask with a placeholder name.
			let res = await api.testSource(input, ctrl.signal);
			// Agent reachable and token accepted (agent info present) but share not (yet) valid: the
			// connection itself is fine, the user picks a share from the list.
			if (input.agent && !res.ok && res.agent && !res.agent.shares?.includes(input.agent.share)) res = { ...res, ok: true, error: undefined };
			draft.test = { ...res, key };
			if (res.ok && draft.kind === 'agent' && draft.agent.mode === 'token') {
				const shares = res.agent?.shares ?? [];
				draft.shares = shares;
				if (!shares.includes(draft.agent.share)) draft.agent.share = shares.length === 1 ? shares[0] : '';
			}
			if (res.ok && draft.kind === 'sftp' && res.hostKey && !draft.sftp.hostKey) draft.sftp.hostKey = res.hostKey;
		} catch (err) {
			if (isAbort(err)) return;
			draft.test = undefined;
			requestError =
				err instanceof ApiError && err.status === 404 && err.code === 'http_404'
					? 'Ce serveur ne sait pas encore tester les sources distantes (mise à jour nécessaire).'
					: errorMessage(err);
		} finally {
			testing = false;
		}
		await tick();
		resultEl?.querySelector<HTMLElement>('[data-focus-after-test]')?.focus();
	}

	/** host_key_unknown → the user confirmed the fingerprint: store it and test again. */
	function trust() {
		if (!fingerprint) return;
		draft.sftp.hostKey = fingerprint;
		test();
	}

	function setAuth(auth: 'password' | 'key') {
		draft.sftp.auth = auth;
		clear('password');
		clear('privateKey');
	}

	const id = (name: string) => `${idp}-${name}`;
	const desc = (field: keyof DraftErrors, hint?: boolean) =>
		[hint ? id(`${field}-hint`) : '', errors[field] ? id(`${field}-err`) : ''].filter(Boolean).join(' ') || undefined;
</script>

{#snippet err(field: keyof DraftErrors)}
	{#if errors[field]}<p class="f-err" id={id(`${field}-err`)}><Icon name="alert" size={14} />{errors[field]}</p>{/if}
{/snippet}

{#snippet nameField(placeholder: string)}
	<div class="f-field">
		<label class="f-label" for={id('name')}>Nom affiché<span class="f-optional">{' (facultatif)'}</span></label>
		<input id={id('name')} class="f-input" type="text" bind:value={draft.name} {placeholder} maxlength="100" autocomplete="off" />
	</div>
{/snippet}

{#snippet testButton(label: string)}
	<div class="test-row">
		<Button
			variant="secondary"
			icon={tested ? 'check' : 'refresh'}
			aria-busy={testing || demoTesting}
			disabled={testing || demoTesting}
			onclick={test}
		>
			{testing || demoTesting ? 'Test en cours…' : label}
		</Button>
		{#if editing && missingSecret && draft.kind !== 'local'}
			<span class="f-hint">Le test demande de saisir à nouveau {draft.kind === 'agent' ? 'le jeton' : draft.sftp.auth === 'password' ? 'le mot de passe' : 'la clé'}.</span>
		{:else if stale && draft.test?.ok}
			<span class="f-hint">Paramètres modifiés depuis le dernier test.</span>
		{/if}
	</div>
{/snippet}

<div class="form" data-source-form={idp}>
	{#if draft.kind === 'local'}
		<div class="f-field">
			<label class="f-label" for={id('root')}>Chemin du dossier</label>
			<input
				id={id('root')}
				class="f-input f-mono"
				type="text"
				placeholder="/srv/media/films"
				bind:value={draft.root}
				oninput={() => clear('root')}
				autocomplete="off"
				spellcheck="false"
				aria-invalid={!!errors.root}
				aria-describedby={desc('root', true)}
			/>
			<p class="f-hint" id={id('root-hint')}>Chemin absolu, tel que le serveur le voit (dans Docker : le chemin dans le conteneur).</p>
			{@render err('root')}
			<div>
				<Button variant="tertiary" icon="folder" onclick={() => (pickerOpen = !pickerOpen)} aria-expanded={pickerOpen} aria-controls={id('picker')}>
					{pickerOpen ? 'Fermer l’explorateur' : 'Parcourir les dossiers du serveur'}
				</Button>
			</div>
		</div>
		{#if pickerOpen}
			<div id={id('picker')}>
				<FolderPicker load={browseServer} bind:value={picked} initialPath={pickerStart} label="Dossiers du serveur" rootsLabel="Serveur" errorText={browseError}>
					{#snippet footer(path)}
						<p class="chosen">{#if path}Dossier choisi : <code>{path}</code>{:else}Ouvrez l’emplacement de vos médias.{/if}</p>
						<Button variant="primary" icon="check" disabled={!path} onclick={() => usePicked(path)}>Utiliser ce dossier</Button>
					{/snippet}
				</FolderPicker>
			</div>
		{/if}
		{@render nameField('Par défaut : le chemin')}
		{@render testButton('Vérifier le dossier')}
	{:else if guided}
		<AgentWizard bind:this={wizard} bind:draft {idp} ontokenmode={() => ((draft.agent.mode = 'token'), (draft.test = undefined))} />
	{:else if paired}
		<div class="paired">
			<span class="p-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
			<span class="p-text">
				<strong>{pairedAgent?.name ?? original?.agent?.hostname ?? 'Agent appairé'}</strong>
				<span>Partage « {draft.agent.share} »{pairedAgent ? ` · ${pairedAgent.url}` : ''}</span>
			</span>
			{#if pairedAgent}<SourceStatusPill status={pairedAgent.status} lastError={pairedAgent.lastError} lastSeen={pairedAgent.lastSeen} />{/if}
		</div>
		<div class="f-field">
			<label class="f-label" for={id('root')}>Sous-dossier<span class="f-optional">{' (facultatif)'}</span></label>
			<input
				id={id('root')}
				class="f-input f-mono"
				type="text"
				placeholder="ex. 4K   (vide = tout le partage)"
				bind:value={draft.root}
				oninput={() => clear('root')}
				autocomplete="off"
				spellcheck="false"
				aria-invalid={!!errors.root}
				aria-describedby={desc('root')}
			/>
			{@render err('root')}
		</div>
		{@render nameField('Par défaut : machine et partage')}
		{@render testButton('Tester la connexion')}
	{:else if draft.kind === 'agent'}
		{#if !editing}
			<p class="f-hint back-guided">
				Jeton manuel (<code>agent.toml</code>).
				<button type="button" class="link" onclick={() => ((draft.agent.mode = 'paired'), (draft.test = undefined))}>Revenir à l’appairage guidé</button>
			</p>
		{/if}
		<div class="f-field">
			<label class="f-label" for={id('url')}>Adresse de l’agent</label>
			<input
				id={id('url')}
				class="f-input f-mono"
				type="url"
				inputmode="url"
				placeholder="http://hote:8097"
				bind:value={draft.agent.url}
				oninput={() => clear('url')}
				autocomplete="off"
				spellcheck="false"
				aria-invalid={!!errors.url}
				aria-describedby={desc('url', true)}
			/>
			<p class="f-hint" id={id('url-hint')}>Machine distante que vous contrôlez, où tourne <code>aether agent</code> (port 8097 par défaut).</p>
			{@render err('url')}
		</div>
		<SecretField
			id={id('token')}
			label="Jeton d’appairage"
			bind:value={draft.agent.token}
			placeholder={editing ? 'Inchangé — laissez vide pour conserver' : 'Collez le jeton affiché par l’agent'}
			hint="Affiché au premier démarrage de l’agent et enregistré dans <data>/agent.token."
			error={errors.token}
		/>
		{@render testButton('Tester la connexion')}
	{:else}
		<div class="host-row">
			<div class="f-field">
				<label class="f-label" for={id('host')}>Hôte</label>
				<input
					id={id('host')}
					class="f-input f-mono"
					type="text"
					placeholder="seedbox.example.net"
					bind:value={draft.sftp.host}
					oninput={() => clear('host')}
					autocomplete="off"
					spellcheck="false"
					autocapitalize="off"
					aria-invalid={!!errors.host}
					aria-describedby={desc('host')}
				/>
				{@render err('host')}
			</div>
			<div class="f-field port">
				<label class="f-label" for={id('port')}>Port</label>
				<input
					id={id('port')}
					class="f-input f-mono tabular"
					type="text"
					inputmode="numeric"
					placeholder="22"
					bind:value={draft.sftp.port}
					oninput={() => clear('port')}
					autocomplete="off"
					aria-invalid={!!errors.port}
					aria-describedby={desc('port')}
				/>
				{@render err('port')}
			</div>
		</div>
		<div class="f-field">
			<label class="f-label" for={id('user')}>Utilisateur</label>
			<input
				id={id('user')}
				class="f-input f-mono"
				type="text"
				bind:value={draft.sftp.user}
				oninput={() => clear('user')}
				autocomplete="off"
				spellcheck="false"
				autocapitalize="off"
				aria-invalid={!!errors.user}
				aria-describedby={desc('user')}
			/>
			{@render err('user')}
		</div>

		<fieldset class="f-field">
			<legend>Authentification</legend>
			<div class="seg">
				<label class:checked={draft.sftp.auth === 'password'}>
					<input type="radio" name="{idp}-auth" value="password" checked={draft.sftp.auth === 'password'} onchange={() => setAuth('password')} />
					<Icon name="lock" size={16} />Mot de passe
				</label>
				<label class:checked={draft.sftp.auth === 'key'}>
					<input type="radio" name="{idp}-auth" value="key" checked={draft.sftp.auth === 'key'} onchange={() => setAuth('key')} />
					<Icon name="key" size={16} />Clé privée
				</label>
			</div>
		</fieldset>

		{#if draft.sftp.auth === 'password'}
			<SecretField
				id={id('password')}
				label="Mot de passe"
				bind:value={draft.sftp.password}
				mono={false}
				autocomplete="new-password"
				placeholder={editing ? 'Inchangé — laissez vide pour conserver' : undefined}
				error={errors.password}
			/>
		{:else}
			<SecretField
				id={id('privateKey')}
				label="Clé privée"
				multiline
				bind:value={draft.sftp.privateKey}
				placeholder={editing ? 'Inchangée — laissez vide pour conserver' : '-----BEGIN OPENSSH PRIVATE KEY-----\n…\n-----END OPENSSH PRIVATE KEY-----'}
				hint="Format PEM ou OpenSSH. Préférez une clé dédiée à Aether, limitée à la lecture."
				error={errors.privateKey}
			/>
			<SecretField
				id={id('passphrase')}
				label="Phrase de passe"
				optional
				mono={false}
				bind:value={draft.sftp.passphrase}
				placeholder={editing ? 'Inchangée — laissez vide pour conserver' : 'Si la clé est chiffrée'}
			/>
		{/if}

		<div class="f-field">
			<label class="f-label" for={id('root')}>Dossier distant</label>
			<input
				id={id('root')}
				class="f-input f-mono"
				type="text"
				placeholder="/home/moi/medias/films"
				bind:value={draft.root}
				oninput={() => clear('root')}
				autocomplete="off"
				spellcheck="false"
				aria-invalid={!!errors.root}
				aria-describedby={desc('root', true)}
			/>
			<p class="f-hint" id={id('root-hint')}>Chemin absolu sur le serveur SFTP.</p>
			{@render err('root')}
		</div>
		{@render nameField('Par défaut : hôte et dossier')}
		{#if draft.sftp.hostKey}
			<p class="f-hint known-key"><Icon name="shield" size={14} />Empreinte enregistrée : <code>{draft.sftp.hostKey}</code></p>
		{/if}
		{@render testButton('Tester la connexion')}
	{/if}

	<!-- Inline result of the last test (announced politely). The guided agent flow shows its own. -->
	{#if !guided}
	<div class="result-slot" role="status" aria-live="polite" bind:this={resultEl}>
		{#if requestError}
			<div class="result bad">
				<Icon name="alert" size={18} />
				<div><strong>Test impossible</strong><p>{requestError}</p></div>
			</div>
		{:else if draft.test && !stale}
			{#if draft.test.ok}
				<div class="result ok">
					<Icon name="check" size={18} />
					<div>
						<strong>Connexion établie</strong>
						{#if draft.kind === 'agent' && draft.test.agent}
							<p>{draft.test.agent.hostname} · agent {draft.test.agent.version} · {draft.test.agent.shares.length} partage{draft.test.agent.shares.length > 1 ? 's' : ''}</p>
						{:else if draft.kind === 'sftp'}
							<p>Dossier accessible, empreinte de l’hôte vérifiée.</p>
						{:else}
							<p>Dossier accessible depuis le serveur.</p>
						{/if}
					</div>
				</div>
			{:else if errCode === 'host_key_unknown' && fingerprint}
				<div class="result trust">
					<span class="t-icon" aria-hidden="true"><Icon name="shield" size={20} /></span>
					<div class="t-body">
						<strong>Première connexion à {draft.sftp.host}</strong>
						<p>Le serveur présente cette empreinte :</p>
						<code class="fp" data-testid="fingerprint">{fingerprint}</code>
						<p>Vérifiez que cette empreinte correspond à celle de votre serveur (fournie par votre hébergeur, ou <code>ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub</code> sur le serveur). Elle sera exigée à chaque connexion, ce qui protège contre l’usurpation.</p>
						<div><Button variant="primary" icon="shield" data-focus-after-test onclick={trust} disabled={testing}>Faire confiance à cet hôte</Button></div>
					</div>
				</div>
			{:else if errCode === 'host_key_mismatch'}
				<div class="result bad">
					<Icon name="alert" size={18} />
					<div class="t-body">
						<strong>L’empreinte de l’hôte a changé</strong>
						<p>{sourceErrorMessage('host_key_mismatch')}</p>
						{#if fingerprint}
							<code class="fp">{fingerprint}</code>
							<div><Button variant="secondary" onclick={trust} disabled={testing}>Faire confiance à la nouvelle empreinte</Button></div>
						{/if}
					</div>
				</div>
			{:else}
				<div class="result bad">
					<Icon name="alert" size={18} />
					<div><strong>Échec de la connexion</strong><p>{sourceErrorMessage(draft.test.error)}</p></div>
				</div>
			{/if}
		{/if}
	</div>
	{/if}

	{#if draft.kind === 'agent' && !paired}
		{#if draft.shares.length}
			<fieldset class="f-field" aria-describedby={errors.share ? id('share-err') : undefined}>
				<legend>Partage</legend>
				<div class="shares">
					{#each draft.shares as share (share)}
						<label class="share" class:checked={draft.agent.share === share}>
							<input type="radio" name="{idp}-share" value={share} bind:group={draft.agent.share} onchange={() => clear('share')} />
							<Icon name="folder" size={16} />{share}
						</label>
					{/each}
				</div>
				{@render err('share')}
			</fieldset>
			<div class="f-field">
				<label class="f-label" for={id('root')}>Sous-dossier<span class="f-optional">{' (facultatif)'}</span></label>
				<input
					id={id('root')}
					class="f-input f-mono"
					type="text"
					placeholder="ex. 4K   (vide = tout le partage)"
					bind:value={draft.root}
					oninput={() => clear('root')}
					autocomplete="off"
					spellcheck="false"
					aria-invalid={!!errors.root}
					aria-describedby={desc('root')}
				/>
				{@render err('root')}
			</div>
			{@render nameField('Par défaut : machine et partage')}
		{:else}
			{@render err('share')}
			<p class="f-hint">Testez la connexion pour choisir parmi les partages déclarés dans <code>agent.toml</code>.</p>
		{/if}
		<AgentHelp legacy />
	{/if}
</div>

<style>
	.form {
		display: grid;
		gap: var(--sp-4);
		min-width: 0;
	}

	.form > :global(*) {
		min-width: 0;
	}

	.host-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 96px;
		gap: var(--sp-3);
		align-items: start;
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.78rem;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.f-hint code {
		font-size: 0.7rem;
	}

	.paired {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--tint), var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		min-width: 0;
	}

	.p-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.p-text {
		display: grid;
		flex: 1;
		min-width: 0;
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.p-text > * {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.p-text strong {
		font-size: var(--fs-body);
		color: var(--text);
	}

	.chosen {
		flex: 1 1 240px;
		min-width: 0;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.back-guided {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 4px;
	}

	.link {
		font-weight: var(--fw-bold);
		color: var(--accent-ink);
		text-decoration: underline;
		text-underline-offset: 3px;
		cursor: pointer;
	}

	.test-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-3);
	}

	.known-key {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
	}

	.seg {
		display: inline-flex;
		gap: 4px;
		width: fit-content;
		padding: 4px;
		border-radius: var(--r-full);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.seg label,
	.share {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 34px;
		padding: 0 14px;
		border-radius: var(--r-full);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.seg label:hover,
	.share:hover {
		color: var(--text);
	}

	.seg label.checked {
		background: var(--surface-active);
		color: var(--text);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 45%, transparent);
	}

	.seg input,
	.share input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.seg label:has(input:focus-visible),
	.share:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.shares {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.share {
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-weight: var(--fw-medium);
	}

	.share.checked {
		background: var(--accent-soft);
		color: var(--accent-ink);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 60%, transparent);
	}

	.result-slot:empty {
		display: none;
	}

	.result {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.result > :global(svg) {
		flex: none;
		margin-top: 1px;
	}

	.result strong {
		display: block;
		color: var(--text);
	}

	.ok {
		background: color-mix(in srgb, var(--success) 12%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--success) 35%, transparent);
	}

	.ok > :global(svg) {
		color: var(--success);
	}

	.bad {
		background: color-mix(in srgb, var(--danger) 10%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent);
	}

	.bad > :global(svg) {
		color: var(--danger);
	}

	.trust {
		background: var(--tint), var(--surface-active);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 45%, transparent);
	}

	.t-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.t-body {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
	}

	.fp {
		display: block;
		padding: var(--sp-2) var(--sp-3);
		border-radius: var(--r-sm);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: 0.8125rem;
		letter-spacing: 0.01em;
		user-select: all;
	}

	@media (max-width: 767px) {
		.host-row {
			grid-template-columns: minmax(0, 1fr) 84px;
		}

		.seg {
			width: 100%;
		}

		.seg label {
			flex: 1;
			justify-content: center;
		}
	}
</style>
