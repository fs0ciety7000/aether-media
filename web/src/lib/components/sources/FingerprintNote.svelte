<script lang="ts">
	// How the server reaches an agent (M5d) and, when pinned, the certificate fingerprint to compare
	// with the one the agent shows (local page, logs). Comparing is optional: pairing already proved the
	// agent saw the same certificate.
	import type { AgentTransport } from '$lib/api/types';
	import { formatFingerprint } from '$lib/sources';
	import Icon from '../Icon.svelte';

	interface Props {
		transport: AgentTransport;
		fingerprint?: string;
		/** Settings list: one line, no explanation. */
		compact?: boolean;
	}

	let { transport, fingerprint, compact = false }: Props = $props();

	const title = $derived(
		transport === 'https-pinned'
			? 'Connexion chiffrée · certificat épinglé'
			: transport === 'https-ca'
				? 'Connexion chiffrée · certificat d’une autorité reconnue'
				: 'Connexion non chiffrée (HTTP)'
	);
</script>

<div class={['fp-note', transport, compact && 'compact']} data-fingerprint={fingerprint ?? ''}>
	{#if !compact}
		<p class="fp-title"><Icon name={transport === 'http' ? 'unlock' : 'lock'} size={16} />{title}</p>
	{/if}
	{#if fingerprint}
		{#if !compact}
			<p class="fp-hint">Empreinte SHA-256 du certificat de l’agent. Pour une vérification facultative, elle doit correspondre à celle affichée par l’agent (page locale ou journaux).</p>
		{/if}
		<p class="fp-line">
			{#if compact}<span class="fp-label"><Icon name="lock" size={12} />Empreinte</span>{/if}
			<code class="fp" aria-label="Empreinte : {formatFingerprint(fingerprint)}">{formatFingerprint(fingerprint)}</code>
		</p>
	{:else if transport === 'http' && !compact}
		<p class="fp-hint">
			Le contenu circule en clair. Agent appairé avant le passage à HTTPS : réappairez-le pour chiffrer la connexion. Agent configuré avec
			<code>tls = false</code> : réservez cet usage à un VPN ou à un proxy HTTPS.
		</p>
	{:else if transport === 'https-ca' && !compact}
		<p class="fp-hint">Le certificat est vérifié comme celui d’un site web (proxy HTTPS ou certificat <code>tls_cert</code> reconnu).</p>
	{/if}
</div>

<style>
	.fp-note {
		display: grid;
		gap: var(--sp-1);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-small);
		color: var(--text-muted);
		min-width: 0;
	}

	.fp-note.http:not(.compact) {
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--danger) 45%, transparent);
	}

	.compact {
		padding: 0;
		background: none;
		box-shadow: none;
	}

	.fp-title {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		font-weight: var(--fw-bold);
		color: var(--text);
	}

	.fp-title :global(svg) {
		flex: none;
	}

	.fp-line {
		display: flex;
		align-items: baseline;
		gap: var(--sp-2);
		flex-wrap: wrap;
		min-width: 0;
	}

	.fp-label {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.fp {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, Consolas, monospace;
		font-size: 0.8rem;
		letter-spacing: 0.03em;
		word-spacing: 0.15em;
		color: var(--text);
		overflow-wrap: anywhere;
	}

	.compact .fp {
		font-size: var(--fs-micro);
	}

	code:not(.fp) {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.78rem;
		color: var(--text);
	}
</style>
