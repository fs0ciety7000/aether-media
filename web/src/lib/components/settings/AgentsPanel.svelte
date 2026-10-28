<script lang="ts">
	// Settings › Libraries › Agents (M5c): paired machines, live status, pair a new one, unpair.
	// Unpairing is refused (409 agent_in_use) while sources use the agent: we say which ones.
	// M5d: transport (HTTPS pinned / CA / HTTP) and certificate fingerprint of each agent; "Réappairer"
	// moves an agent paired over HTTP to HTTPS, or accepts a certificate that changed legitimately.
	import { onMount } from 'svelte';
	import { api, ApiError, errorMessage } from '$lib/api/client';
	import { agents, libraries } from '$lib/api/stores.svelte';
	import type { Agent } from '$lib/api/types';
	import { formatRelative } from '$lib/format';
	import { emptyDraft, hostOf, sourceErrorMessage, TRANSPORT_LABEL, type SourceDraft } from '$lib/sources';
	import { toasts } from '$lib/toast.svelte';
	import Button from '../Button.svelte';
	import ConfirmDialog from '../ConfirmDialog.svelte';
	import Dialog from '../Dialog.svelte';
	import Icon from '../Icon.svelte';
	import Skeleton from '../Skeleton.svelte';
	import AgentWizard from '../sources/AgentWizard.svelte';
	import FingerprintNote from '../sources/FingerprintNote.svelte';
	import SourceStatusPill from '../sources/SourceStatusPill.svelte';

	interface Props {
		now?: number;
	}

	let { now }: Props = $props();

	let pairOpen = $state(false);
	let pairDraft = $state<SourceDraft>(emptyDraft('agent'));
	/** Agent being paired again (null: a new one). */
	let repairing = $state<Agent | null>(null);
	let toUnpair = $state<Agent | null>(null);
	let unpairOpen = $state(false);
	let unpairing = $state(false);
	/** 409: the agent is still used — shown in the dialog instead of closing it. */
	let inUse = $state<string | null>(null);

	onMount(() => {
		agents.ensure();
	});

	/** Sources (from the libraries store) that use an agent: "Films · salon · films". */
	function usedBy(a: Agent): string[] {
		const out: string[] = [];
		for (const lib of libraries.list) for (const s of lib.sources) if (s.agent?.agentId === a.id) out.push(`${s.name} (${lib.name})`);
		return out;
	}

	function openPair() {
		pairDraft = emptyDraft('agent');
		repairing = null;
		pairOpen = true;
	}

	function openRepair(a: Agent) {
		pairDraft = emptyDraft('agent');
		repairing = a;
		pairOpen = true;
	}

	function paired(a: Agent) {
		pairOpen = false;
		const again = !!repairing;
		repairing = null;
		agents.refresh();
		toasts.show(again ? `Agent « ${a.name} » réappairé` : `Agent « ${a.name} » appairé`, {
			detail: again ? 'Ses sources utilisent désormais la nouvelle connexion.' : 'Ajoutez une source « Agent Aether » pour partager ses dossiers.'
		});
	}

	function askUnpair(a: Agent) {
		toUnpair = a;
		inUse = null;
		unpairOpen = true;
	}

	async function confirmUnpair() {
		if (!toUnpair) return;
		const a = toUnpair;
		unpairing = true;
		try {
			await api.deleteAgent(a.id);
			agents.remove(a.id);
			unpairOpen = false;
			toasts.show(`Agent « ${a.name} » désappairé`, { detail: 'Son jeton pour ce serveur a été révoqué.' });
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				const list = usedBy(a);
				inUse = list.length
					? `Cet agent est encore utilisé par ${list.length === 1 ? 'la source' : `${list.length} sources`} : ${list.join(', ')}. Supprimez-${list.length === 1 ? 'la' : 'les'} d’abord, puis réessayez.`
					: 'Cet agent est encore utilisé par des sources : supprimez-les d’abord, puis réessayez.';
				agents.refresh();
			} else {
				unpairOpen = false;
				toasts.show('Désappairage impossible', { detail: errorMessage(err) });
			}
		} finally {
			unpairing = false;
		}
	}

	function meta(a: Agent): string {
		const parts = [hostOf(a.url)];
		if (a.version) parts.push(`agent ${a.version}`);
		parts.push(a.sources ? `${a.sources} source${a.sources > 1 ? 's' : ''}` : 'aucune source');
		const seen = formatRelative(a.lastSeen, now ?? Date.now());
		if (a.status !== 'online' && seen) parts.push(`vu ${seen}`);
		return parts.join(' · ');
	}
</script>

<section class="agents panel" aria-labelledby="agents-title">
	<header class="a-head">
		<div>
			<h2 id="agents-title">Agents</h2>
			<p class="lead">Machines appairées qui partagent des dossiers avec ce serveur (NAS, PC du salon…). Lancez <code>aether agent</code> dessus, puis appairez-les par code.</p>
		</div>
		<Button variant="secondary" icon="plus" onclick={openPair}>Appairer un agent</Button>
	</header>

	{#if !agents.loaded}
		<Skeleton height="64px" radius="lg" />
	{:else if agents.error && !agents.list.length}
		<p class="msg" role="alert"><Icon name="alert" size={16} />Liste des agents indisponible : {errorMessage(agents.error)}</p>
	{:else if !agents.list.length}
		<p class="msg">
			<Icon name="server" size={16} />Aucun agent appairé. Ajoutez une source « Agent Aether » à une bibliothèque, ou appairez une machine dès maintenant.
		</p>
	{:else}
		<ul class="list" role="list">
			{#each agents.list as a (a.id)}
				<li class={['agent', a.status]} data-agent-id={a.id}>
					<span class="a-icon" aria-hidden="true"><Icon name="server" size={18} /></span>
					<div class="a-text">
						<p class="a-name">{a.name}</p>
						<p class="a-meta">{meta(a)}</p>
						<p class={['a-transport', a.transport]}>
							<Icon name={a.transport === 'http' ? 'unlock' : 'lock'} size={12} />{TRANSPORT_LABEL[a.transport]}{#if a.transport === 'http'}
								— réappairez pour chiffrer{/if}
						</p>
						{#if a.fingerprint}<FingerprintNote transport={a.transport} fingerprint={a.fingerprint} compact />{/if}
						{#if (a.status === 'offline' || a.status === 'cert_mismatch') && a.lastError}<p class="a-err"><Icon name="alert" size={13} />{sourceErrorMessage(a.lastError)}</p>{/if}
					</div>
					<span class="a-status"><SourceStatusPill status={a.status} lastError={a.lastError} lastSeen={a.lastSeen} {now} /></span>
					<span class="a-actions">
						<Button
							variant={a.status === 'cert_mismatch' || a.transport === 'http' ? 'secondary' : 'tertiary'}
							icon="key"
							onclick={() => openRepair(a)}
							aria-label="Réappairer {a.name}"><span class="lbl">Réappairer</span></Button
						>
						<Button variant="tertiary" icon="unlock" onclick={() => askUnpair(a)} aria-label="Désappairer {a.name}"><span class="lbl">Désappairer</span></Button>
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<Dialog
	bind:open={pairOpen}
	title={repairing ? `Réappairer « ${repairing.name} »` : 'Appairer un agent'}
	description={repairing
		? 'Saisissez le code affiché par l’agent : le serveur reçoit un nouveau jeton et enregistre le certificat actuel (HTTPS).'
		: 'Trouvez la machine sur le réseau ou saisissez son adresse, puis tapez le code qu’elle affiche.'}
	size="lg"
>
	{#if pairOpen}
		{#key repairing?.id}
			<AgentWizard bind:draft={pairDraft} idp="pair" pairOnly onpaired={paired} repairAgent={repairing ?? undefined} />
		{/key}
	{/if}
	{#snippet footer()}
		<Button variant="tertiary" onclick={() => (pairOpen = false)}>Fermer</Button>
	{/snippet}
</Dialog>

<ConfirmDialog bind:open={unpairOpen} title="Désappairer l’agent ?" confirmLabel="Désappairer" danger busy={unpairing} onconfirm={confirmUnpair}>
	<p>
		Le serveur oubliera « <strong>{toUnpair?.name}</strong> » et son jeton sera révoqué sur l’agent. Les fichiers de la machine ne sont pas
		touchés ; vous pourrez l’appairer de nouveau avec un code.
	</p>
	{#if inUse}
		<p class="in-use" role="alert"><Icon name="alert" size={16} /><span>{inUse}</span></p>
	{:else if toUnpair?.sources}
		<p class="warn">
			<Icon name="info" size={16} /><span>{toUnpair.sources === 1 ? 'Une source utilise' : `${toUnpair.sources} sources utilisent`} encore cet agent : le serveur refusera tant qu’elles existent.</span>
		</p>
	{/if}
</ConfirmDialog>

<style>
	.agents {
		display: grid;
		gap: var(--sp-4);
		padding: var(--sp-5);
	}

	.a-head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sp-4);
		flex-wrap: wrap;
	}

	.lead {
		margin-top: var(--sp-1);
		max-width: 62ch;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.78rem;
		color: var(--text);
	}

	.msg {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.msg :global(svg) {
		flex: none;
	}

	.list {
		display: grid;
		gap: var(--sp-2);
	}

	.agent {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto auto;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.a-icon {
		display: grid;
		place-items: center;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.offline .a-icon,
	.cert_mismatch .a-icon {
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text-muted);
	}

	.a-text {
		display: grid;
		gap: 1px;
		min-width: 0;
	}

	.a-name {
		font-weight: var(--fw-bold);
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.a-meta {
		font-size: var(--fs-micro);
		color: var(--text-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.a-transport {
		display: flex;
		align-items: center;
		gap: 5px;
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.a-transport.http {
		color: var(--danger);
	}

	.a-err {
		display: flex;
		align-items: flex-start;
		gap: 5px;
		margin-top: 2px;
		font-size: var(--fs-micro);
		color: var(--danger);
	}

	.in-use,
	.warn {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		margin-top: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-md);
		font-size: var(--fs-small);
	}

	.in-use {
		background: color-mix(in srgb, var(--danger) 10%, var(--surface));
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent);
		color: var(--text);
	}

	.in-use :global(svg) {
		flex: none;
		margin-top: 2px;
		color: var(--danger);
	}

	.warn {
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.warn :global(svg) {
		flex: none;
		margin-top: 2px;
	}

	@media (max-width: 767px) {
		.agents {
			padding: var(--sp-4);
		}

		.a-head :global(.btn) {
			width: 100%;
		}

		.agent {
			grid-template-columns: auto minmax(0, 1fr);
			grid-template-areas: 'icon text' 'icon status' 'icon actions';
			row-gap: var(--sp-2);
			align-items: start;
		}

		.a-icon {
			grid-area: icon;
		}

		.a-text {
			grid-area: text;
		}

		.a-status {
			grid-area: status;
			justify-self: start;
		}

		.a-actions {
			grid-area: actions;
			justify-self: start;
			display: flex;
			flex-wrap: wrap;
			gap: var(--sp-2);
		}
	}
</style>
