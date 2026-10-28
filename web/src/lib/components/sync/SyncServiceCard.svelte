<script lang="ts">
	// One synchronisation service for the signed-in user: pairing (device code / PIN, polled),
	// account and state, "Synchroniser maintenant", features, disconnection and the journal.
	// The page owns the list (and the live `sync.status` updates); changes go back through `onupdate`.
	import { onDestroy, onMount, untrack } from 'svelte';
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import type { DeviceCode, SyncConnection, SyncEvent, SyncFeature } from '$lib/api/types';
	import { formatDate, formatRelative } from '$lib/format';
	import { FEATURES, SERVICE_INFO } from '$lib/sync';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import ConfirmDialog from '../ConfirmDialog.svelte';
	import Icon from '../Icon.svelte';
	import Menu, { type MenuItem } from '../Menu.svelte';
	import Switch from '../Switch.svelte';
	import Tag from '../Tag.svelte';
	import DeviceCodePanel, { type PairingPhase } from './DeviceCodePanel.svelte';
	import ServiceMark from './ServiceMark.svelte';
	import SyncJournal from './SyncJournal.svelte';

	interface Props {
		connection: SyncConnection;
		isAdmin: boolean;
		onupdate: (c: SyncConnection) => void;
	}

	let { connection: c, isAdmin, onupdate }: Props = $props();

	const info = $derived(SERVICE_INFO[c.service]);
	const linked = $derived(c.status === 'connected' || c.status === 'reauth_required');
	const uid = $props.id();

	let now = $state(Date.now());

	// ——— Pairing ———
	let pairing = $state<{ code: DeviceCode; phase: PairingPhase; lifetime: number } | null>(null);
	let connecting = $state(false);
	let interval = 5;
	let pollTimer: ReturnType<typeof setTimeout> | undefined;
	let pollCtrl: AbortController | undefined;
	let destroyed = false;

	function stopPolling() {
		clearTimeout(pollTimer);
		pollTimer = undefined;
		pollCtrl?.abort();
		pollCtrl = undefined;
	}

	function schedule(seconds: number) {
		clearTimeout(pollTimer);
		if (destroyed) return;
		pollTimer = setTimeout(pollOnce, Math.max(1, seconds) * 1000);
	}

	function begin(code: DeviceCode) {
		interval = Math.max(1, code.interval || 5);
		const left = new Date(code.expiresAt).getTime() - Date.now();
		pairing = { code, phase: 'waiting', lifetime: Math.max(left, 1000) };
		now = Date.now();
		schedule(interval);
	}

	async function connect() {
		if (connecting) return;
		stopPolling();
		connecting = true;
		try {
			begin(await api.syncConnect(c.service));
		} catch (err) {
			if (err instanceof ApiError && err.code === 'sync_unavailable') onupdate({ ...c, available: false });
			toasts.show(`Connexion à ${info.name} impossible`, { detail: errorMessage(err) });
		} finally {
			connecting = false;
		}
	}

	async function pollOnce() {
		const p = pairing;
		if (!p || p.phase === 'expired' || p.phase === 'denied') return;
		if (new Date(p.code.expiresAt).getTime() <= Date.now()) {
			p.phase = 'expired';
			return;
		}
		pollCtrl = new AbortController();
		try {
			const r = await api.syncPoll(c.service, pollCtrl.signal);
			if (pairing !== p) return;
			interval = Math.max(interval, r.interval || interval);
			switch (r.status) {
				case 'connected':
					pairing = null;
					onupdate(r.connection);
					toasts.show(`Compte ${info.name} relié`, {
						detail: r.connection.account?.username ? `Connecté en tant que ${r.connection.account.username}. Import initial en cours…` : 'Import initial en cours…'
					});
					return;
				case 'expired':
					p.phase = 'expired';
					return;
				case 'denied':
					p.phase = 'denied';
					return;
				case 'slow_down':
					p.phase = 'slow';
					schedule(interval);
					return;
				default:
					schedule(interval);
			}
		} catch (err) {
			if (pairing !== p || destroyed) return;
			if (err instanceof DOMException && err.name === 'AbortError') return;
			if (err instanceof ApiError && err.code === 'not_pending') {
				p.phase = 'expired';
				return;
			}
			// Network blip or service hiccup: keep trying (slower), the code is still valid.
			interval = Math.min(interval * 2, 30);
			schedule(interval);
		} finally {
			pollCtrl = undefined;
		}
	}

	function cancelPairing() {
		stopPolling();
		pairing = null;
		// No cancel endpoint: the pending code simply expires server-side.
		dismissedPending = c.pending?.userCode;
	}

	/** Pending code from the server we chose to hide (Annuler): not resumed on refresh. */
	let dismissedPending = $state<string | undefined>();

	onMount(() => {
		// Pairing started earlier (other tab, reload): resume it.
		if (c.pending && new Date(c.pending.expiresAt).getTime() > Date.now()) begin(c.pending);
		if (linked) loadEvents();
		const t = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(t);
	});

	onDestroy(() => {
		destroyed = true;
		stopPolling();
	});

	// The countdown hit zero: say so without waiting for the next poll.
	$effect(() => {
		const p = pairing;
		if (p && (p.phase === 'waiting' || p.phase === 'slow') && new Date(p.code.expiresAt).getTime() <= now) {
			untrack(() => {
				stopPolling();
				p.phase = 'expired';
			});
		}
	});

	// ——— Journal ———
	let events = $state<SyncEvent[] | null>(null);
	let eventsError = $state('');

	let eventsLoading = false;
	async function loadEvents() {
		if (eventsLoading) return;
		eventsLoading = true;
		try {
			events = await api.syncEvents(c.service, 50);
			eventsError = '';
		} catch (err) {
			events ??= [];
			eventsError = errorMessage(err);
		} finally {
			eventsLoading = false;
		}
	}

	// A synchronisation just finished (live sync.status): refresh the journal.
	let wasSyncing = untrack(() => c.syncing);
	$effect(() => {
		const s = c.syncing;
		const st = c.status;
		untrack(() => {
			if (wasSyncing && !s && (st === 'connected' || st === 'reauth_required')) loadEvents();
			if (st === 'connected' && events === null) loadEvents();
			wasSyncing = s;
		});
	});

	// ——— Actions ———
	let running = $state(false);

	async function run(full = false) {
		if (running) return;
		running = true;
		try {
			await api.syncRun(c.service, full);
			onupdate({ ...c, syncing: true });
			toasts.show(full ? 'Resynchronisation complète demandée' : 'Synchronisation demandée', {
				detail: full ? `Tout est comparé avec ${info.name} ; cela peut prendre quelques minutes.` : undefined,
				duration: 3500
			});
		} catch (err) {
			if (err instanceof ApiError && err.code === 'reauth_required') onupdate({ ...c, status: 'reauth_required' });
			toasts.show('Synchronisation impossible', { detail: errorMessage(err) });
		} finally {
			running = false;
		}
	}

	async function setFeature(key: SyncFeature, value: boolean) {
		const before = c.features;
		onupdate({ ...c, features: { ...c.features, [key]: value } });
		try {
			onupdate(await api.syncFeatures(c.service, { [key]: value }));
		} catch (err) {
			onupdate({ ...c, features: before });
			toasts.show('Modification impossible', { detail: errorMessage(err) });
		}
	}

	let disconnectOpen = $state(false);
	let disconnecting = $state(false);

	async function disconnect() {
		disconnecting = true;
		try {
			await api.syncDisconnect(c.service);
			disconnectOpen = false;
			stopPolling();
			pairing = null;
			events = null;
			onupdate({
				...c,
				status: 'disconnected',
				account: undefined,
				syncing: false,
				connectedAt: undefined,
				lastSyncAt: undefined,
				lastError: undefined,
				lastErrorAt: undefined,
				pending: undefined
			});
			toasts.show(`${info.name} déconnecté`, { detail: 'Vos données dans Aether (vus, notes, Ma liste) sont conservées.' });
		} catch (err) {
			toasts.show('Déconnexion impossible', { detail: errorMessage(err) });
		} finally {
			disconnecting = false;
		}
	}

	const menuItems = $derived<MenuItem[]>([
		{ label: 'Resynchronisation complète', icon: 'refresh', onselect: () => run(true), disabled: c.syncing || c.status !== 'connected' },
		{ label: 'Relier un autre compte…', icon: 'link', onselect: connect, disabled: !!pairing },
		{ label: `Déconnecter ${info.name}…`, icon: 'log-out', onselect: () => (disconnectOpen = true), separated: true }
	]);

	const statusTag = $derived.by(() => {
		if (!c.available && !linked) return { label: 'Non configuré', variant: 'surface' as const };
		if (c.status === 'reauth_required') return { label: 'Reconnexion nécessaire', variant: 'surface' as const, icon: 'alert' as const };
		if (c.status === 'connected') return { label: 'Connecté', variant: 'accent' as const, icon: 'check' as const };
		if (pairing) return { label: 'Appairage en cours', variant: 'surface' as const };
		return { label: 'Non connecté', variant: 'surface' as const };
	});

	const lastSync = $derived(c.lastSyncAt ? formatRelative(c.lastSyncAt, now) : undefined);
	const features = $derived(FEATURES.filter((f) => c.supported.includes(f.key)));
	const showPairing = $derived(!!pairing && pairing.code.userCode !== dismissedPending);
</script>

<article class="card panel" class:reauth={c.status === 'reauth_required'} aria-labelledby="svc-{uid}">
	<header class="head">
		<ServiceMark name={info.name} size={44} muted={!c.available && !linked} />
		<div class="title">
			<h2 id="svc-{uid}">{info.name}</h2>
			<p class="site">{info.site}</p>
		</div>
		<Tag variant={statusTag.variant} icon={statusTag.icon}>{statusTag.label}</Tag>
		{#if c.status === 'connected'}
			<div class="head-actions">
				<Button icon="refresh" aria-busy={c.syncing || running} disabled={c.syncing || running} onclick={() => run(false)}>
					{c.syncing ? 'Synchronisation…' : 'Synchroniser maintenant'}
				</Button>
				<Menu label="Plus d’options pour {info.name}" items={menuItems} />
			</div>
		{:else if c.status === 'reauth_required'}
			<div class="head-actions"><Menu label="Plus d’options pour {info.name}" items={menuItems} /></div>
		{/if}
	</header>

	{#if !linked}
		<p class="pitch">{info.pitch}</p>
	{/if}

	{#if !c.available && !linked}
		<div class="note" role="status">
			<Icon name="lock" size={18} />
			<div>
				<p><strong>{info.name} n’est pas encore configuré sur ce serveur.</strong></p>
				{#if isAdmin}
					<p>Enregistrez d’abord l’application {info.name} d’Aether (identifiants d’application) dans les réglages.</p>
				{:else}
					<p>Demandez à un administrateur d’enregistrer l’application {info.name} dans les réglages d’Aether.</p>
				{/if}
			</div>
			{#if isAdmin}<Button href="/reglages/synchronisation" icon="settings">Configurer {info.name}</Button>{/if}
		</div>
	{:else}
		{#if c.status === 'reauth_required' && !showPairing}
			<div class="banner" role="alert">
				<Icon name="alert" size={20} />
				<div>
					<p><strong>{info.name} n’accepte plus l’accès d’Aether.</strong></p>
					<p>Le jeton a expiré ou a été révoqué. Reconnectez {c.account?.username ? `le compte ${c.account.username}` : 'votre compte'} pour reprendre la synchronisation ; vos réglages sont conservés.</p>
				</div>
				<Button variant="primary" icon="link" disabled={connecting} aria-busy={connecting} onclick={connect}>Reconnecter</Button>
			</div>
		{/if}

		{#if showPairing && pairing}
			<DeviceCodePanel
				code={pairing.code}
				service={info.name}
				phase={pairing.phase}
				lifetime={pairing.lifetime}
				{now}
				busy={connecting}
				oncancel={cancelPairing}
				onrestart={connect}
			/>
		{:else if c.status === 'disconnected' || c.status === 'pending'}
			<div class="connect">
				<Button variant="primary" icon="link" disabled={connecting} aria-busy={connecting} onclick={connect}>Connecter {info.name}</Button>
				<p class="micro hint">Un code s’affiche ici : saisissez-le sur {info.site}. Aether ne voit jamais votre mot de passe.</p>
			</div>
		{/if}

		{#if linked}
			<dl class="facts">
				<div>
					<dt>Compte</dt>
					<dd class="account">{c.account?.username ?? '—'}</dd>
				</div>
				<div>
					<dt>Dernière synchronisation</dt>
					<dd>
						{#if c.syncing}
							<span class="syncing"><span class="spin" aria-hidden="true"><Icon name="refresh" size={14} /></span>En cours…</span>
						{:else}
							<span title={c.lastSyncAt ? formatDate(c.lastSyncAt) : undefined}>{lastSync ?? 'jamais'}</span>
						{/if}
					</dd>
				</div>
				{#if c.connectedAt}
					<div>
						<dt>Relié depuis</dt>
						<dd>{formatDate(c.connectedAt)}</dd>
					</div>
				{/if}
			</dl>
			<p class="visually-hidden" role="status">{c.syncing ? `Synchronisation ${info.name} en cours` : ''}</p>

			{#if c.lastError && c.status === 'connected'}
				<p class="last-error" role="status">
					<Icon name="alert" size={16} />
					<span>
						<strong>Dernière erreur{c.lastErrorAt ? ` · ${formatRelative(c.lastErrorAt, now)}` : ''}</strong>
						{c.lastError}
					</span>
				</p>
			{/if}

			{#if features.length}
				<section class="features" aria-labelledby="feat-{uid}">
					<h3 id="feat-{uid}">Éléments synchronisés</h3>
					<div class="switches">
						{#each features as f (f.key)}
							<div class="sw">
								<Switch label={f.label} hint={f.hint[c.service]} checked={!!c.features[f.key]} onchange={(v) => setFeature(f.key, v)} />
							</div>
						{/each}
					</div>
				</section>
			{/if}

			<details class="log" open>
				<summary>
					<span>Journal</span>
					{#if events?.length}<span class="count tabular">{events.length}</span>{/if}
					<span class="chev" aria-hidden="true"><Icon name="chevron-down" size={16} /></span>
				</summary>
				<SyncJournal {events} service={c.service} error={eventsError} {now} />
			</details>
		{/if}
	{/if}
</article>

<ConfirmDialog bind:open={disconnectOpen} title="Déconnecter {info.name} ?" confirmLabel="Déconnecter" danger busy={disconnecting} onconfirm={disconnect}>
	<p>
		Aether arrête de synchroniser avec {c.account?.username ? `le compte ${info.name} « ${c.account.username} »` : `votre compte ${info.name}`}
		{c.service === 'trakt' ? ' et révoque son accès' : ''}. Le journal est effacé.
	</p>
	<p>Vos données dans Aether (vus, progression, notes, Ma liste) sont conservées, comme celles sur {info.site}.</p>
</ConfirmDialog>

<style>
	.card {
		display: grid;
		gap: var(--sp-5);
		padding: var(--sp-5) var(--sp-6) var(--sp-6);
		min-width: 0;
	}

	.card.reauth {
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent);
	}

	.head {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-3) var(--sp-4);
	}

	.title {
		display: grid;
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

	.head-actions {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		margin-left: auto;
	}

	.pitch {
		max-width: 68ch;
		color: var(--text-muted);
	}

	.note,
	.banner {
		display: flex;
		align-items: center;
		gap: var(--sp-3) var(--sp-4);
		flex-wrap: wrap;
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: color-mix(in srgb, var(--bg) 45%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.note > div,
	.banner > div {
		display: grid;
		gap: 2px;
		flex: 1;
		min-width: min(100%, 260px);
	}

	.note strong,
	.banner strong {
		color: var(--text);
	}

	.note > :global(svg) {
		flex: none;
		align-self: flex-start;
		margin-top: 2px;
		color: var(--text-faint);
	}

	.banner {
		background: color-mix(in srgb, var(--danger) 9%, var(--bg-sunken));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 30%, transparent);
	}

	.banner > :global(svg) {
		flex: none;
		align-self: flex-start;
		margin-top: 1px;
		color: var(--danger);
	}

	.connect {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--sp-3) var(--sp-4);
	}

	.hint {
		color: var(--text-muted);
		max-width: 52ch;
	}

	.facts {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, max-content));
		gap: var(--sp-2) var(--sp-8);
	}

	.facts div {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	dt {
		font-size: var(--fs-micro);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		margin: 0;
		color: var(--text);
		font-weight: var(--fw-medium);
		overflow-wrap: anywhere;
	}

	.account {
		font-weight: var(--fw-bold);
	}

	.syncing {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		color: var(--accent-ink);
	}

	.spin {
		display: inline-flex;
		animation: spin 1.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.spin {
			animation: none;
		}
	}

	.last-error {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--danger) 8%, transparent);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.last-error :global(svg) {
		flex: none;
		margin-top: 2px;
		color: var(--danger);
	}

	.last-error strong {
		display: block;
		color: var(--text);
	}

	.features {
		display: grid;
		gap: var(--sp-3);
	}

	h3 {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.switches {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--sp-2);
	}

	.sw {
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: color-mix(in srgb, var(--bg) 40%, transparent);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.log {
		border-top: 1px solid var(--line);
		padding-top: var(--sp-3);
	}

	summary {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		min-height: 36px;
		margin-bottom: var(--sp-2);
		border-radius: var(--r-xs);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
		cursor: pointer;
		list-style: none;
	}

	summary::-webkit-details-marker {
		display: none;
	}

	.count {
		min-width: 20px;
		padding: 0 6px;
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 8%, transparent);
		font-size: var(--fs-micro);
		line-height: 20px;
		text-align: center;
	}

	.chev {
		display: inline-flex;
		transition: transform var(--dur-fast) var(--ease);
	}

	.log[open] .chev {
		transform: rotate(180deg);
	}

	.log:not([open]) summary {
		margin-bottom: 0;
	}

	@media (max-width: 767px) {
		.card {
			padding: var(--sp-5) var(--sp-4);
		}

		.head-actions {
			width: 100%;
			margin-left: 0;
		}

		.head-actions :global(.btn) {
			flex: 1;
		}

		.switches {
			grid-template-columns: minmax(0, 1fr);
		}

		.facts {
			grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
			gap: var(--sp-3) var(--sp-5);
		}

		.connect :global(.btn) {
			width: 100%;
		}
	}
</style>
