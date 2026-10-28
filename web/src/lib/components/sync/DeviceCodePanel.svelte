<script lang="ts" module>
	export type PairingPhase = 'waiting' | 'slow' | 'expired' | 'denied';
</script>

<script lang="ts">
	// Device pairing (Trakt device code, Simkl PIN): open the site, type the code, wait.
	// Purely presentational: the polling lives with the caller.
	import type { DeviceCode } from '$lib/api/types';
	import { formatCountdown } from '$lib/sync';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';
	import CopyCode from './CopyCode.svelte';

	interface Props {
		code: DeviceCode;
		service: string;
		phase: PairingPhase;
		/** Current time (ms), ticked by the caller. */
		now: number;
		/** Total validity at the start (ms), for the bar. */
		lifetime?: number;
		busy?: boolean;
		oncancel: () => void;
		onrestart: () => void;
	}

	let { code, service, phase, now, lifetime, busy = false, oncancel, onrestart }: Props = $props();

	const left = $derived(Math.max(0, new Date(code.expiresAt).getTime() - now));
	const ratio = $derived(lifetime ? Math.min(1, left / lifetime) : 1);
	const ended = $derived(phase === 'expired' || phase === 'denied');
	const host = $derived(code.verificationUrl.replace(/^https?:\/\//, '').replace(/\/$/, ''));
	// Announced once a minute (and in the last 10 s) rather than every second.
	const spoken = $derived(left > 60_000 ? `${Math.ceil(left / 60_000)} minutes` : left > 10_000 ? 'moins d’une minute' : 'quelques secondes');
</script>

<div class="pairing" class:ended>
	<ol class="steps">
		<li>
			<span class="n" aria-hidden="true">1</span>
			<div class="body">
				<p>Ouvrez la page d’activation de {service} et connectez-vous.</p>
				{#if ended}
					<span class="host">{host}</span>
				{:else}
					<Button href={code.verificationUrl} target="_blank" rel="noopener noreferrer" icon="external">
						{host}<span class="visually-hidden"> (nouvel onglet)</span>
					</Button>
				{/if}
			</div>
		</li>
		<li>
			<span class="n" aria-hidden="true">2</span>
			<div class="body">
				<p>Saisissez ce code :</p>
				<CopyCode code={code.userCode} label="Code à saisir sur {host}" stale={ended} />
			</div>
		</li>
	</ol>

	<div class="status" role="status">
		{#if phase === 'expired'}
			<p class="state"><Icon name="clock" size={18} /><span><strong>Le code a expiré.</strong> Demandez-en un nouveau pour relier votre compte.</span></p>
		{:else if phase === 'denied'}
			<p class="state"><Icon name="alert" size={18} /><span><strong>Accès refusé sur {service}.</strong> Rien n’a été relié ; vous pouvez réessayer.</span></p>
		{:else}
			<p class="state waiting">
				<span class="spin" aria-hidden="true"><Icon name="refresh" size={18} /></span>
				<span>
					En attente de la validation sur {service}…
					{#if phase === 'slow'}<span class="muted"> Le service demande de patienter un peu entre deux vérifications.</span>{/if}
				</span>
			</p>
			<p class="timer tabular" aria-hidden="true">Expire dans {formatCountdown(left)}</p>
			<span class="visually-hidden">Le code expire dans {spoken}.</span>
		{/if}
	</div>
	{#if !ended}
		<div class="bar" aria-hidden="true"><span style:transform="scaleX({ratio})"></span></div>
	{/if}

	<div class="actions">
		{#if ended}
			<Button variant="primary" icon="refresh" disabled={busy} onclick={onrestart}>Nouveau code</Button>
			<Button variant="tertiary" onclick={oncancel}>Fermer</Button>
		{:else}
			<Button variant="tertiary" onclick={oncancel}>Annuler</Button>
		{/if}
	</div>
</div>

<style>
	.pairing {
		display: grid;
		gap: var(--sp-4);
		padding: var(--sp-5);
		border-radius: var(--r-lg);
		background: color-mix(in srgb, var(--bg) 45%, transparent);
		box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 28%, transparent);
	}

	.pairing.ended {
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.steps {
		display: grid;
		gap: var(--sp-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.steps li {
		display: flex;
		gap: var(--sp-3);
		min-width: 0;
	}

	.n {
		display: grid;
		place-items: center;
		flex: none;
		width: 26px;
		height: 26px;
		border-radius: 50%;
		background: var(--surface-active);
		color: var(--accent-ink);
		font-size: var(--fs-micro);
		font-weight: var(--fw-heavy);
	}

	.body {
		display: grid;
		justify-items: start;
		gap: var(--sp-2);
		min-width: 0;
		padding-top: 3px;
	}

	.body p {
		color: var(--text-muted);
	}

	.host {
		font-weight: var(--fw-bold);
		color: var(--text-faint);
	}

	.status {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-wrap: wrap;
		gap: var(--sp-2) var(--sp-4);
	}

	.state {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		color: var(--text);
		font-size: var(--fs-small);
	}

	.state :global(svg) {
		flex: none;
		margin-top: 1px;
		color: var(--text-muted);
	}

	.waiting :global(svg) {
		color: var(--accent);
	}

	.muted {
		color: var(--text-muted);
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

	.timer {
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.bar {
		height: 3px;
		margin-top: calc(-1 * var(--sp-2));
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 8%, transparent);
		overflow: hidden;
	}

	.bar span {
		display: block;
		height: 100%;
		background: var(--accent);
		transform-origin: left;
		transition: transform 1s linear;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	@media (max-width: 479px) {
		.pairing {
			padding: var(--sp-4);
		}

		.actions :global(.btn) {
			flex: 1;
		}
	}
</style>
