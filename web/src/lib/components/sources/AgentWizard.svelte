<script lang="ts">
	// Guided pairing of an Aether agent (M5c), in the spirit of Jellyfin: no agent.toml to write.
	//   1. Agent: a paired one (live status), or a new one found on the network / typed by address
	//   2. Code: the XXXX-XXXX code shown by the agent (skipped for an agent already paired)
	//   3. Folder: browse the agent's folders, share the chosen one (POST /agents/{id}/shares)
	//   4. Check: POST /sources/test on { agentId, share }; the parent then saves the source.
	// `pairOnly` stops after step 2 (Settings › Agents: pair a machine without creating a source).
	// M5d: after pairing, the pinned certificate fingerprint is shown for an optional comparison
	// with the agent's; `repairAgent` pairs an existing agent again (HTTP → HTTPS, new certificate).
	import { onDestroy, onMount, tick, untrack } from 'svelte';
	import { api, isAbort } from '$lib/api/client';
	import { agents } from '$lib/api/stores.svelte';
	import type { Agent, AgentShare, DiscoveredAgent } from '$lib/api/types';
	import {
		agentErrorMessage,
		draftToInput,
		formatFingerprint,
		formatPairCode,
		hostOf,
		isTested,
		pairCodeProblem,
		parseAgentAddress,
		sourceErrorMessage,
		testKey,
		type SourceDraft
	} from '$lib/sources';
	import Button from '../Button.svelte';
	import FolderPicker from '../FolderPicker.svelte';
	import Icon from '../Icon.svelte';
	import Skeleton from '../Skeleton.svelte';
	import Stepper, { type Step as StepItem } from '../Stepper.svelte';
	import AgentHelp from './AgentHelp.svelte';
	import FingerprintNote from './FingerprintNote.svelte';
	import SourceStatusPill from './SourceStatusPill.svelte';

	interface Props {
		draft: SourceDraft;
		idp?: string;
		/** Pair a machine only (no folder, no source). */
		pairOnly?: boolean;
		onpaired?: (a: Agent) => void;
		/** Switch to the manual token form (agent.toml + agent.token). */
		ontokenmode?: () => void;
		/** Pair this agent again (starts at the code step). */
		repairAgent?: Agent;
	}

	let { draft = $bindable(), idp = 'agent', pairOnly = false, onpaired, ontokenmode, repairAgent }: Props = $props();

	type StepId = 'agent' | 'code' | 'folder' | 'check' | 'done';

	let step = $state<StepId>(
		untrack(() => (repairAgent ? 'code' : draft.agent.agentId ? (draft.agent.share ? 'check' : 'folder') : 'agent'))
	);
	/** The agent was picked from the paired list: the code step is skipped. */
	let codeSkipped = $state(untrack(() => !!draft.agent.agentId));
	let headEl = $state<HTMLElement>();

	// — Step 1: agent —
	let discovering = $state(false);
	let discovered = $state<DiscoveredAgent[] | null>(null);
	let discoverError = $state<string | null>(null);
	let manualOpen = $state(false);
	let address = $state('');
	let addressError = $state<string | null>(null);
	let discoverCtrl: AbortController | null = null;

	// — Step 2: code —
	let target = $state<{ url: string; name?: string; detail?: string; repair?: boolean; agentId?: number } | null>(
		untrack(() => (repairAgent ? { url: repairAgent.url, name: repairAgent.name, detail: hostOf(repairAgent.url), repair: true, agentId: repairAgent.id } : null))
	);
	let code = $state('');
	let codeError = $state<string | null>(null);
	/** API code of the last pairing failure (certificate problems get their own explanation). */
	let codeErrorKind = $state('');
	let pairing = $state(false);
	/** Agent just paired here: its certificate fingerprint is shown for comparison. */
	let justPaired = $state<Agent | null>(null);

	// — Step 3: folder —
	let shares = $state<AgentShare[] | null>(null);
	let folder = $state('');
	let browseCode = $state('');
	let sharing = $state(false);
	let shareError = $state<string | null>(null);

	// — Step 4: check —
	let testing = $state(false);
	let testError = $state<string | null>(null);
	let testCtrl: AbortController | null = null;

	const agent = $derived(agents.get(draft.agent.agentId));
	const known = $derived(agents.list);

	onMount(() => {
		agents.ensure().then(() => {
			// Nothing paired yet: look on the network straight away.
			if (step === 'agent' && (pairOnly || !agents.list.length) && discovered === null) discover();
		});
		if (step === 'folder') loadShares();
		if (step === 'check' && !isTested(draft)) verify();
	});

	onDestroy(() => {
		discoverCtrl?.abort();
		testCtrl?.abort();
	});

	async function go(next: StepId) {
		step = next;
		await tick();
		headEl?.focus();
	}

	// ——— Step 1 ———

	async function discover() {
		discoverCtrl?.abort();
		const c = (discoverCtrl = new AbortController());
		discovering = true;
		discoverError = null;
		try {
			discovered = await api.discoverAgents(c.signal);
		} catch (err) {
			if (isAbort(err)) return;
			discovered = [];
			discoverError = agentErrorMessage(err);
		} finally {
			if (discoverCtrl === c) discovering = false;
		}
	}

	function selectAgent(a: Agent) {
		draft.agent.mode = 'paired';
		draft.agent.agentId = a.id;
		draft.agent.agentName = a.name;
		draft.agent.share = '';
		draft.agent.sharePath = undefined;
		draft.root = '';
		draft.test = undefined;
		shares = null;
		folder = '';
		browseCode = '';
		go('folder');
		loadShares();
	}

	async function pickDiscovered(d: DiscoveredAgent) {
		if (d.paired && d.agentId !== undefined && !pairOnly) {
			let a = agents.get(d.agentId);
			if (!a) {
				await agents.refresh();
				a = agents.get(d.agentId);
			}
			if (a) {
				codeSkipped = true;
				return selectAgent(a);
			}
		}
		startPairing({ url: d.url, name: d.name, detail: `${d.hostname} · ${d.addr}:${d.port}`, repair: d.paired, agentId: d.paired ? d.agentId : undefined });
	}

	function pickKnown(a: Agent) {
		codeSkipped = true;
		selectAgent(a);
	}

	function startPairing(t: NonNullable<typeof target>) {
		target = t;
		code = '';
		codeError = null;
		codeErrorKind = '';
		codeSkipped = false;
		go('code');
	}

	function submitAddress() {
		const r = parseAgentAddress(address);
		if ('error' in r) {
			addressError = r.error;
			document.getElementById(`${idp}-addr`)?.focus();
			return;
		}
		addressError = null;
		const k = known.find((a) => a.url.replace(/\/+$/, '') === r.url);
		startPairing({ url: r.url, name: k?.name, detail: hostOf(r.url), repair: !!k, agentId: k?.id });
	}

	async function toggleManual() {
		manualOpen = !manualOpen;
		if (manualOpen) {
			await tick();
			document.getElementById(`${idp}-addr`)?.focus();
		}
	}

	// ——— Step 2 ———

	function onCodeInput(e: Event) {
		const el = e.currentTarget as HTMLInputElement;
		code = formatPairCode(el.value);
		el.value = code;
		codeError = null;
	}

	async function pair() {
		if (!target || pairing) return;
		const problem = pairCodeProblem(code);
		if (problem) {
			codeError = problem;
			document.getElementById(`${idp}-code`)?.focus();
			return;
		}
		pairing = true;
		codeError = null;
		codeErrorKind = '';
		try {
			const a = await api.pairAgent({ url: target.url, code, agentId: target.agentId });
			agents.upsert(a);
			justPaired = a;
			if (pairOnly) {
				go('done');
				return;
			}
			selectAgent(a);
		} catch (err) {
			codeErrorKind = (err as { code?: string })?.code ?? '';
			codeError = agentErrorMessage(err, 'pair');
			await tick();
			document.getElementById(`${idp}-code`)?.focus();
		} finally {
			pairing = false;
		}
	}

	// ——— Step 3 ———

	async function loadShares() {
		const id = draft.agent.agentId;
		if (!id) return;
		try {
			const list = await api.agentShares(id);
			if (draft.agent.agentId === id) shares = list;
		} catch {
			shares = [];
		}
	}

	const browse = $derived.by(() => {
		const id = draft.agent.agentId;
		return async (path: string, signal: AbortSignal) => {
			if (!id) throw new Error('no agent');
			try {
				const res = await api.agentBrowse(id, path, signal);
				browseCode = '';
				return res;
			} catch (err) {
				browseCode = (err as { code?: string })?.code ?? '';
				throw err;
			}
		};
	});

	const existing = $derived(shares?.find((s) => samePath(s.path, folder)));

	function samePath(a: string, b: string) {
		const t = (p: string) => (p.length > 1 ? p.replace(/[\\/]+$/, '') : p);
		return !!a && !!b && t(a) === t(b);
	}

	function useShare(s: AgentShare) {
		draft.agent.share = s.name;
		draft.agent.sharePath = s.path;
		draft.root = '';
		draft.test = undefined;
		go('check');
		verify();
	}

	async function shareFolder() {
		const id = draft.agent.agentId;
		if (!id || !folder || sharing) return;
		if (existing) return useShare(existing);
		sharing = true;
		shareError = null;
		try {
			const s = await api.addAgentShare(id, { path: folder });
			shares = [...(shares ?? []).filter((x) => x.name !== s.name), s];
			useShare(s);
		} catch (err) {
			shareError = agentErrorMessage(err, 'share');
		} finally {
			sharing = false;
		}
	}

	function repair() {
		if (!agent) return;
		startPairing({ url: agent.url, name: agent.name, detail: hostOf(agent.url), repair: true, agentId: agent.id });
	}

	/** Pairing refused because of the certificate (interception, proxy): explained apart from a wrong code. */
	const certProblem = $derived(['agent_fingerprint_mismatch', 'agent_proof_invalid', 'agent_cert_untrusted'].includes(codeErrorKind));

	// ——— Step 4 ———

	/** Runs the connection test on { agentId, share }; the parent enables "Add" when it passes. */
	export async function verify() {
		if (step !== 'check' || !draft.agent.agentId || !draft.agent.share) return;
		testCtrl?.abort();
		const c = (testCtrl = new AbortController());
		testing = true;
		testError = null;
		const key = testKey(draft);
		try {
			const res = await api.testSource(draftToInput(draft), c.signal);
			if (res.ok && res.agent?.hostname && !draft.agent.agentName) draft.agent.agentName = res.agent.hostname;
			draft.test = { ...res, key };
		} catch (err) {
			if (isAbort(err)) return;
			draft.test = undefined;
			testError = agentErrorMessage(err);
		} finally {
			if (testCtrl === c) testing = false;
		}
	}

	// ——— Stepper ———

	const order: StepId[] = $derived(pairOnly ? ['agent', 'code'] : ['agent', 'code', 'folder', 'check']);
	const stepItems: StepItem[] = $derived(
		[
			{ label: 'Agent', onclick: () => go('agent') },
			{ label: 'Code', skipped: codeSkipped && step !== 'code' && step !== 'agent' },
			{ label: 'Dossier', onclick: () => go('folder') },
			{ label: 'Vérification' }
		].slice(0, order.length)
	);
	const current = $derived(step === 'done' ? order.length : order.indexOf(step));

	const id = (n: string) => `${idp}-${n}`;
	const localPage = $derived.by(() => {
		try {
			const u = new URL(target?.url ?? '');
			return `https://localhost:${u.port || (u.protocol === 'https:' ? '443' : '80')}`;
		} catch {
			return 'https://localhost:8097';
		}
	});
	const tested = $derived(isTested(draft));
</script>

<div class="wizard" data-agent-wizard={idp}>
	<Stepper steps={stepItems} {current} label="Étapes de l’ajout d’un agent" />

	{#if step === 'agent'}
		<h4 class="s-head" tabindex="-1" bind:this={headEl}>{pairOnly ? 'Quelle machine appairer ?' : 'Quelle machine partage les fichiers ?'}</h4>

		{#if !pairOnly}
			<section class="group" aria-labelledby={id('known-t')}>
				<h5 class="g-title" id={id('known-t')}>Agents appairés</h5>
				{#if !agents.loaded}
					<div class="cards" aria-hidden="true"><Skeleton height="64px" radius="lg" /></div>
				{:else if known.length}
					<ul class="cards" role="list">
						{#each known as a (a.id)}
							<li>
								<button type="button" class={['card', a.status]} onclick={() => pickKnown(a)} data-agent-id={a.id}>
									<span class="c-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
									<span class="c-text">
										<strong>{a.name}</strong>
										<span>{hostOf(a.url)}{a.version ? ` · agent ${a.version}` : ''}{a.sources ? ` · ${a.sources} source${a.sources > 1 ? 's' : ''}` : ''}</span>
									</span>
									<SourceStatusPill status={a.status} lastError={a.lastError} lastSeen={a.lastSeen} />
									<span class="c-go" aria-hidden="true"><Icon name="chevron-right" size={18} /></span>
								</button>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="muted">Aucun agent appairé pour l’instant : ajoutez-en un ci-dessous.</p>
				{/if}
			</section>
		{/if}

		<section class="group" aria-labelledby={id('new-t')}>
			<h5 class="g-title" id={id('new-t')}>{pairOnly ? 'Sur le réseau' : 'Nouvel agent'}</h5>
			<div class="actions">
				<Button variant={known.length && !pairOnly ? 'secondary' : 'primary'} icon="search" onclick={discover} disabled={discovering} aria-busy={discovering}>
					{discovering ? 'Recherche…' : discovered ? 'Relancer la recherche' : 'Rechercher sur le réseau'}
				</Button>
				<Button variant="tertiary" icon="edit" onclick={toggleManual} aria-expanded={manualOpen} aria-controls={id('manual')}>Saisir une adresse</Button>
			</div>

			{#if manualOpen}
				<div class="manual" id={id('manual')}>
					<div class="f-field">
						<label class="f-label" for={id('addr')}>Adresse de l’agent</label>
						<div class="inline">
							<input
								id={id('addr')}
								class="f-input f-mono"
								type="text"
								inputmode="url"
								placeholder="192.168.1.20 ou https://nas.local:8097"
								bind:value={address}
								oninput={() => (addressError = null)}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										submitAddress();
									}
								}}
								autocomplete="off"
								spellcheck="false"
								autocapitalize="off"
								aria-invalid={!!addressError}
								aria-describedby="{id('addr-hint')}{addressError ? ` ${id('addr-err')}` : ''}"
							/>
							<Button variant="secondary" icon="arrow-right" onclick={submitAddress}>Continuer</Button>
						</div>
						<p class="f-hint" id={id('addr-hint')}>
							Port 8097 par défaut ; HTTPS est utilisé dès que l’agent le propose. Utile à travers Internet (seedbox), un VPN, ou quand Aether tourne
							dans Docker sans réseau « host ».
						</p>
						{#if addressError}<p class="f-err" id={id('addr-err')}><Icon name="alert" size={14} />{addressError}</p>{/if}
					</div>
				</div>
			{/if}

			<div class="discovery" role="status" aria-live="polite" aria-busy={discovering}>
				{#if discovering}
					<div class="scan">
						<span class="pulse" aria-hidden="true"></span>
						<span>Recherche des agents sur le réseau local…</span>
					</div>
				{:else if discoverError}
					<div class="result bad">
						<Icon name="alert" size={18} />
						<div><strong>Recherche impossible</strong><p>{discoverError}</p></div>
					</div>
				{:else if discovered && discovered.length}
					<p class="muted">{discovered.length === 1 ? '1 agent trouvé' : `${discovered.length} agents trouvés`}</p>
					<ul class="cards" role="list">
						{#each discovered as d (d.uid)}
							<li>
								<button type="button" class="card" onclick={() => pickDiscovered(d)} data-discovered={d.uid}>
									<span class="c-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
									<span class="c-text">
										<strong>{d.name}</strong>
										<span>{d.hostname !== d.name ? `${d.hostname} · ` : ''}{d.addr}:{d.port}{d.version ? ` · agent ${d.version}` : ''}</span>
									</span>
									{#if d.tls}<span class="tag" title={d.fingerprint ? `HTTPS · empreinte annoncée ${formatFingerprint(d.fingerprint)}` : 'Connexion chiffrée (HTTPS)'}
											><Icon name="lock" size={13} />HTTPS</span
										>{/if}
									{#if d.paired}<span class="tag on"><Icon name="check" size={13} />Appairé</span>{/if}
									<span class="c-go" aria-hidden="true"><Icon name="chevron-right" size={18} /></span>
								</button>
							</li>
						{/each}
					</ul>
				{:else if discovered}
					<div class="empty">
						<span class="e-icon" aria-hidden="true"><Icon name="search" size={20} /></span>
						<div>
							<strong>Aucun agent trouvé sur le réseau local</strong>
							<ul>
								<li>Vérifiez que <code>aether agent</code> tourne sur la machine qui possède les fichiers.</li>
								<li>Les deux machines doivent être sur le même réseau local : la recherche ne traverse ni les VPN ni les routeurs.</li>
								<li>Le pare-feu doit laisser passer le port UDP 7360. Si Aether tourne dans Docker, la recherche demande <code>network_mode: host</code>.</li>
							</ul>
							<div class="actions">
								<Button variant="secondary" icon="refresh" onclick={discover}>Relancer</Button>
								{#if !manualOpen}<Button variant="tertiary" icon="edit" onclick={toggleManual}>Saisir une adresse</Button>{/if}
							</div>
						</div>
					</div>
				{/if}
			</div>
		</section>

		<AgentHelp />
		{#if ontokenmode && !pairOnly}
			<p class="f-hint legacy">
				Agent déjà configuré avec <code>agent.toml</code> et un jeton ?
				<button type="button" class="link" onclick={ontokenmode}>Utiliser un jeton manuel</button>
			</p>
		{/if}
	{:else if step === 'code' && target}
		<h4 class="s-head" tabindex="-1" bind:this={headEl}>{target.repair ? 'Appairer de nouveau' : 'Code d’appairage'}</h4>
		<div class="target">
			<span class="c-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
			<span class="c-text">
				<strong>{target.name ?? hostOf(target.url)}</strong>
				<span><code>{target.url}</code></span>
			</span>
			<Button variant="tertiary" onclick={() => go('agent')}>Changer</Button>
		</div>

		<div class="f-field">
			<label class="f-label" for={id('code')}>Code affiché par l’agent</label>
			<div class="inline">
				<input
					id={id('code')}
					class="f-input code-input"
					type="text"
					value={code}
					oninput={onCodeInput}
					onkeydown={(e) => {
						if (e.key === 'Enter') {
							e.preventDefault();
							pair();
						}
					}}
					placeholder="XXXX-XXXX"
					maxlength="9"
					autocomplete="one-time-code"
					autocapitalize="characters"
					spellcheck="false"
					aria-invalid={!!codeError}
					aria-describedby="{id('code-hint')}{codeError ? ` ${id('code-err')}` : ''}"
				/>
				<Button variant="primary" icon="key" onclick={pair} disabled={pairing} aria-busy={pairing}>{pairing ? 'Appairage…' : 'Appairer'}</Button>
			</div>
			<p class="f-hint" id={id('code-hint')}>8 caractères, valable 10 minutes. Il change après chaque appairage. Le code ne circule jamais en clair.</p>
			{#if codeError && !certProblem}<p class="f-err" id={id('code-err')} role="alert"><Icon name="alert" size={14} />{codeError}</p>{/if}
		</div>
		{#if codeError && certProblem}
			<div class="result bad" role="alert" id={id('code-err')} data-pair-error={codeErrorKind}>
				<Icon name="shield" size={18} />
				<div class="t-body">
					<strong>Appairage refusé : certificat inattendu</strong>
					<p>{codeError}</p>
					{#if codeErrorKind === 'agent_fingerprint_mismatch'}
						<p>
							Un proxy, un pare-feu qui inspecte le trafic ou un intermédiaire malveillant présente son propre certificat à la place de celui de
							l’agent. Vérifiez que l’adresse mène directement à l’agent (ou passez par un tunnel SSH / VPN), puis saisissez le nouveau code.
						</p>
					{/if}
				</div>
			</div>
		{/if}

		<div class="where">
			<p class="w-title"><Icon name="info" size={16} />Où trouver le code ?</p>
			<ol>
				<li>
					<strong>Sur la page d’état de l’agent</strong> : sur la machine de l’agent elle-même, ouvrez
					<code>{localPage}</code> dans un navigateur. Cette page n’est servie qu’en local.
				</li>
				<li>
					<strong>Dans ses journaux</strong>, au démarrage puis toutes les 10 minutes :
					<code class="log">Code d'appairage : K7PQ-4XRM (valable 10 min)</code>
					(terminal où tourne <code>aether agent</code>, <code>journalctl -u aether-agent</code> ou <code>docker logs</code>).
				</li>
			</ol>
		</div>
	{:else if step === 'done' && justPaired}
		<h4 class="s-head" tabindex="-1" bind:this={headEl}>Agent appairé</h4>
		<div class="result ok" role="status">
			<Icon name="check" size={18} />
			<div><strong>« {justPaired.name} » est appairé</strong><p><code>{justPaired.url}</code></p></div>
		</div>
		<FingerprintNote transport={justPaired.transport} fingerprint={justPaired.fingerprint} />
		<div class="actions"><Button variant="primary" icon="check" onclick={() => justPaired && onpaired?.(justPaired)}>Terminer</Button></div>
	{:else if step === 'folder' && draft.agent.agentId}
		<h4 class="s-head" tabindex="-1" bind:this={headEl}>Quel dossier partager ?</h4>
		{#if justPaired && justPaired.id === draft.agent.agentId}
			<FingerprintNote transport={justPaired.transport} fingerprint={justPaired.fingerprint} />
		{/if}
		<div class="target">
			<span class="c-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
			<span class="c-text">
				<strong>{agent?.name ?? draft.agent.agentName ?? 'Agent'}</strong>
				<span>{agent ? hostOf(agent.url) : ''}{agent?.version ? ` · agent ${agent.version}` : ''}</span>
			</span>
			{#if agent}<SourceStatusPill status={agent.status} lastError={agent.lastError} lastSeen={agent.lastSeen} />{/if}
			<Button variant="tertiary" onclick={() => go('agent')}>Changer</Button>
		</div>

		{#if browseCode === 'agent_unpaired'}
			<div class="result bad" role="alert">
				<Icon name="alert" size={18} />
				<div class="t-body">
					<strong>Appairage révoqué</strong>
					<p>Cet agent ne reconnaît plus ce serveur (son état a été réinitialisé, ou le serveur a été retiré depuis sa page locale).</p>
					<div><Button variant="primary" icon="key" onclick={repair}>Appairer de nouveau</Button></div>
				</div>
			</div>
		{/if}

		{#if shares && shares.length}
			<div class="f-field">
				<p class="f-label">Déjà partagés</p>
				<div class="chips">
					{#each shares as s (s.name)}
						<button type="button" class="chip" onclick={() => useShare(s)} title={s.path}>
							<Icon name="folder" size={15} /><span class="chip-name">{s.name}</span><span class="chip-path">{s.path}</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<FolderPicker load={browse} bind:value={folder} label="Dossiers de l’agent" rootsLabel="Emplacements" errorText={(e) => agentErrorMessage(e, 'browse')}>
			{#snippet footer(path)}
				<p class="chosen">
					{#if path}Dossier choisi : <code>{path}</code>{:else}Ouvrez un emplacement puis le dossier de vos médias.{/if}
				</p>
				<Button variant="primary" icon={existing ? 'check' : 'folder'} disabled={!path || sharing} aria-busy={sharing} onclick={shareFolder}>
					{sharing ? 'Partage…' : existing ? `Utiliser « ${existing.name} »` : 'Partager ce dossier'}
				</Button>
			{/snippet}
		</FolderPicker>
		{#if shareError}<p class="f-err" role="alert"><Icon name="alert" size={14} />{shareError}</p>{/if}
	{:else if step === 'check'}
		<h4 class="s-head" tabindex="-1" bind:this={headEl}>Vérification</h4>
		<div class="summary">
			<div class="target">
				<span class="c-icon" aria-hidden="true"><Icon name="server" size={20} /></span>
				<span class="c-text">
					<strong>{agent?.name ?? draft.agent.agentName ?? 'Agent'}</strong>
					<span>Partage « {draft.agent.share} »{draft.agent.sharePath ? ' · ' : ''}{#if draft.agent.sharePath}<code>{draft.agent.sharePath}</code>{/if}</span>
				</span>
				<Button variant="tertiary" onclick={() => go('folder')}>Changer de dossier</Button>
			</div>
		</div>

		<div class="result-slot" role="status" aria-live="polite">
			{#if testing}
				<div class="scan"><span class="pulse" aria-hidden="true"></span><span>Test de la connexion à l’agent…</span></div>
			{:else if testError}
				<div class="result bad">
					<Icon name="alert" size={18} />
					<div class="t-body">
						<strong>Test impossible</strong>
						<p>{testError}</p>
						<div><Button variant="secondary" icon="refresh" onclick={verify}>Réessayer</Button></div>
					</div>
				</div>
			{:else if draft.test && tested}
				<div class="result ok">
					<Icon name="check" size={18} />
					<div>
						<strong>Connexion établie</strong>
						{#if draft.test.agent}<p>{draft.test.agent.hostname} · agent {draft.test.agent.version} · le dossier est lisible.</p>{/if}
					</div>
				</div>
			{:else if draft.test && !draft.test.ok}
				<div class="result bad">
					<Icon name="alert" size={18} />
					<div class="t-body">
						<strong>Échec de la connexion</strong>
						<p>{sourceErrorMessage(draft.test.error)}</p>
						<div class="actions">
							<Button variant="secondary" icon="refresh" onclick={verify}>Réessayer</Button>
							<Button variant="tertiary" onclick={() => go('folder')}>Choisir un autre dossier</Button>
						</div>
					</div>
				</div>
			{/if}
		</div>

		<div class="f-field">
			<label class="f-label" for={id('name')}>Nom affiché<span class="f-optional">{' (facultatif)'}</span></label>
			<input id={id('name')} class="f-input" type="text" bind:value={draft.name} placeholder="Par défaut : machine et dossier" maxlength="100" autocomplete="off" />
		</div>
	{/if}
</div>

<style>
	.wizard {
		display: grid;
		gap: var(--sp-4);
		min-width: 0;
	}

	.wizard > :global(*) {
		min-width: 0;
	}

	.s-head {
		font-size: var(--fs-h3);
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.s-head:focus {
		outline: none;
	}

	.group {
		display: grid;
		gap: var(--sp-3);
	}

	.g-title {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.muted {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.cards {
		display: grid;
		gap: var(--sp-2);
	}

	.card,
	.target {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		text-align: left;
		min-width: 0;
	}

	.card {
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease);
	}

	.card:hover {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.target {
		background: var(--tint), var(--surface);
	}

	.c-icon,
	.e-icon {
		display: grid;
		place-items: center;
		flex: none;
		width: 40px;
		height: 40px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.card.offline .c-icon {
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.c-text {
		display: grid;
		gap: 1px;
		flex: 1;
		min-width: 0;
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.c-text > * {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.c-text strong {
		font-size: var(--fs-body);
		color: var(--text);
	}

	.c-go {
		display: inline-grid;
		color: var(--text-muted);
	}

	.tag {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		flex: none;
		height: 24px;
		padding: 0 9px;
		border-radius: var(--r-full);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.tag.on {
		background: var(--accent-soft);
		box-shadow: none;
		color: var(--accent-ink);
	}

	.manual {
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.inline {
		display: flex;
		gap: var(--sp-2);
		align-items: center;
	}

	.inline input {
		flex: 1;
		min-width: 0;
	}

	.discovery:empty {
		display: none;
	}

	.discovery {
		display: grid;
		gap: var(--sp-2);
	}

	.scan {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.pulse {
		position: relative;
		flex: none;
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: var(--accent);
		animation: pulse 1.2s var(--ease) infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.35;
			transform: scale(0.8);
		}
		50% {
			opacity: 1;
			transform: scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.pulse {
			animation: none;
		}
	}

	.empty {
		display: flex;
		gap: var(--sp-3);
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.empty > div {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
	}

	.empty strong {
		color: var(--text);
	}

	.empty ul {
		display: grid;
		gap: 4px;
		padding-left: 1.1em;
		list-style: disc;
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.78rem;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.code-input {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 1.25rem;
		font-weight: var(--fw-bold);
		letter-spacing: 0.18em;
		text-transform: uppercase;
		max-width: 16ch;
		height: 48px;
	}

	.code-input::placeholder {
		letter-spacing: 0.18em;
	}

	.where {
		display: grid;
		gap: var(--sp-2);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.w-title {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.where ol {
		display: grid;
		gap: var(--sp-2);
		padding-left: 1.2em;
	}

	.where strong {
		color: var(--text);
	}

	.log {
		display: block;
		width: fit-content;
		max-width: 100%;
		margin: 4px 0;
		padding: 4px 8px;
		border-radius: var(--r-xs);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
		min-width: 0;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		max-width: 100%;
		min-width: 0;
		overflow: hidden;
		height: 34px;
		padding: 0 14px 0 10px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		font-size: var(--fs-small);
		color: var(--text-muted);
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease);
	}

	.chip:hover {
		background: var(--surface-active);
		color: var(--text);
	}

	.chip :global(svg) {
		flex: none;
		color: var(--accent-ink);
	}

	.chip-name {
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.chip-path {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.75rem;
	}

	.chosen {
		flex: 1 1 240px;
		min-width: 0;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.legacy {
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

	.t-body {
		display: grid;
		gap: var(--sp-2);
		min-width: 0;
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

	@media (max-width: 767px) {
		.card :global(.pill) {
			display: none;
		}

		.card.offline :global(.pill),
		.target :global(.pill) {
			display: inline-flex;
		}

		.inline {
			flex-wrap: wrap;
		}

		.inline input {
			flex: 1 1 100%;
		}

		.code-input {
			max-width: none;
		}

		.target {
			flex-wrap: wrap;
		}
	}
</style>
