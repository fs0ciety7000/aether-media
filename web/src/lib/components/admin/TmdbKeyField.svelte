<script lang="ts">
	// TMDB API key (assistant and Réglages › Serveur): checked live against TMDB before it is saved.
	// The key never comes back from the server (only "…ab12"); a key from the configuration is read-only.
	import { api, errorMessage } from '$lib/api/client';
	import type { SecretSetting, ServerSettings, TmdbCheck } from '$lib/api/types';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';
	import SecretField from '../sources/SecretField.svelte';
	import SettingMeta from './SettingMeta.svelte';

	interface Props {
		setting: SecretSetting;
		/** Called with the new settings after a save or a removal. */
		onsaved?: (s: ServerSettings) => void;
		/** Open the help on how to get a key. */
		helpOpen?: boolean;
		idp?: string;
	}

	let { setting, onsaved, helpOpen = false, idp = 'tmdb' }: Props = $props();

	let editing = $state(false);
	let key = $state('');
	let busy = $state<'' | 'check' | 'save' | 'remove'>('');
	let result = $state<{ tone: 'ok' | 'error' | 'warn'; text: string } | null>(null);
	/** TMDB could not be reached: offer to save anyway. */
	let unreachable = $state(false);

	const showInput = $derived(!setting.readOnly && (!setting.set || editing));

	function describe(r: TmdbCheck): { tone: 'ok' | 'error' | 'warn'; text: string } {
		if (r.valid) return { tone: 'ok', text: 'Clé valide : TMDB a accepté la connexion.' };
		if (r.error === 'invalid_key')
			return { tone: 'error', text: 'TMDB refuse cette clé. Copiez la « Clé d’API » (v3) ou le « Jeton d’accès en lecture » (v4) depuis votre compte TMDB.' };
		if (r.error === 'missing') return { tone: 'warn', text: 'Aucune clé à vérifier.' };
		return { tone: 'warn', text: 'TMDB est injoignable depuis le serveur. Vérifiez sa connexion à Internet, ou enregistrez la clé et réessayez plus tard.' };
	}

	async function checkStored() {
		busy = 'check';
		result = null;
		try {
			result = describe(await api.checkTmdb());
		} catch (err) {
			result = { tone: 'error', text: errorMessage(err) };
		} finally {
			busy = '';
		}
	}

	async function save(e?: SubmitEvent, force = false) {
		e?.preventDefault();
		const k = key.trim();
		result = null;
		unreachable = false;
		if (!k) {
			result = { tone: 'error', text: 'Collez votre clé TMDB.' };
			return;
		}
		if (!force) {
			busy = 'check';
			try {
				const r = await api.checkTmdb(k);
				if (!r.valid) {
					result = describe(r);
					unreachable = r.error === 'unreachable';
					return;
				}
			} catch (err) {
				result = { tone: 'error', text: errorMessage(err) };
				return;
			} finally {
				busy = '';
			}
		}
		busy = 'save';
		try {
			const s = await api.patchSettings({ tmdbApiKey: k });
			key = '';
			editing = false;
			result = { tone: 'ok', text: force ? 'Clé enregistrée sans vérification.' : 'Clé valide et enregistrée : les métadonnées TMDB sont actives.' };
			onsaved?.(s);
		} catch (err) {
			result = { tone: 'error', text: errorMessage(err) };
		} finally {
			busy = '';
		}
	}

	async function remove() {
		busy = 'remove';
		result = null;
		try {
			const s = await api.patchSettings({ tmdbApiKey: '' });
			result = { tone: 'warn', text: 'Clé retirée : TMDB est désactivé.' };
			onsaved?.(s);
		} catch (err) {
			result = { tone: 'error', text: errorMessage(err) };
		} finally {
			busy = '';
		}
	}
</script>

<div class="tmdb">
	{#if setting.readOnly}
		<div class="state">
			<span class="badge" aria-hidden="true"><Icon name="lock" size={18} /></span>
			<div class="state-text">
				<strong>Clé définie par la configuration</strong>
				<span>{setting.set ? `Clé ${setting.masked ?? ''}` : 'Aucune clé'} · modifiez <code>AETHER_TMDB_API_KEY</code> ou <code>metadata.tmdb_api_key</code> puis redémarrez Aether pour la changer.</span>
			</div>
			{#if setting.set}<Button size="md" icon="check-circle" disabled={!!busy} onclick={checkStored}>{busy === 'check' ? 'Vérification…' : 'Vérifier'}</Button>{/if}
		</div>
	{:else if !showInput}
		<div class="state">
			<span class="badge ok" aria-hidden="true"><Icon name="key" size={18} /></span>
			<div class="state-text">
				<strong>Clé enregistrée</strong>
				<span class="mono">{setting.masked ?? '…'}</span>
			</div>
			<div class="state-actions">
				<Button icon="check-circle" disabled={!!busy} onclick={checkStored}>{busy === 'check' ? 'Vérification…' : 'Vérifier'}</Button>
				<Button variant="tertiary" icon="edit" disabled={!!busy} onclick={() => ((editing = true), (result = null))}>Remplacer</Button>
				<Button variant="tertiary" icon="trash" disabled={!!busy} onclick={remove}>{busy === 'remove' ? 'Retrait…' : 'Retirer'}</Button>
			</div>
		</div>
	{:else}
		<form class="form" onsubmit={save} novalidate>
			<SecretField
				id="{idp}-key"
				label="Clé d’API TMDB"
				bind:value={key}
				placeholder="Clé v3 (32 caractères) ou jeton v4"
				hint="Elle reste sur votre serveur, chiffrée, et n’est jamais réaffichée."
				error={result?.tone === 'error' ? result.text : undefined}
			/>
			<div class="actions">
				<Button variant="primary" type="submit" icon="check" disabled={!!busy}>
					{busy === 'check' ? 'Vérification auprès de TMDB…' : busy === 'save' ? 'Enregistrement…' : 'Vérifier et enregistrer'}
				</Button>
				{#if unreachable}<Button variant="tertiary" disabled={!!busy} onclick={() => save(undefined, true)}>Enregistrer sans vérifier</Button>{/if}
				{#if editing}<Button variant="tertiary" disabled={!!busy} onclick={() => ((editing = false), (key = ''), (result = null))}>Annuler</Button>{/if}
			</div>
		</form>
	{/if}

	{#if result && !(showInput && result.tone === 'error')}
		<p class={['result', result.tone]} role={result.tone === 'ok' ? 'status' : 'alert'}>
			<Icon name={result.tone === 'ok' ? 'check-circle' : 'alert'} size={16} />{result.text}
		</p>
	{/if}

	<SettingMeta source={setting.source} readOnly={setting.readOnly} applies={setting.applies} env="AETHER_TMDB_API_KEY" />

	<details class="help" open={helpOpen}>
		<summary><Icon name="info" size={16} />Obtenir une clé gratuite<span class="chev" aria-hidden="true"><Icon name="chevron-down" size={16} /></span></summary>
		<ol>
			<li>
				Créez un compte gratuit sur
				<a href="https://www.themoviedb.org/signup" target="_blank" rel="noopener noreferrer">themoviedb.org<Icon name="external" size={13} /><span class="visually-hidden"> (nouvel onglet)</span></a>.
			</li>
			<li>
				Ouvrez
				<a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noopener noreferrer">Paramètres › API<Icon name="external" size={13} /><span class="visually-hidden"> (nouvel onglet)</span></a>
				et demandez une clé « Developer » (usage personnel).
			</li>
			<li>Copiez la « Clé d’API » (ou le « Jeton d’accès en lecture ») et collez-la ici.</li>
		</ol>
		<p class="note">Sans clé, Aether lit les fichiers NFO et les images posées à côté de vos vidéos ; la musique utilise MusicBrainz, qui n’en demande pas.</p>
	</details>
</div>

<style>
	.tmdb {
		display: grid;
		gap: var(--sp-3);
	}

	.state {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.badge {
		display: grid;
		place-items: center;
		flex: none;
		width: 38px;
		height: 38px;
		border-radius: var(--r-md);
		background: var(--accent-soft);
		color: var(--accent-ink);
	}

	.state-text {
		display: grid;
		gap: 2px;
		flex: 1 1 220px;
		min-width: 0;
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.state-text strong {
		color: var(--text);
		font-size: var(--fs-body);
	}

	.state-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.mono,
	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.8125rem;
		color: var(--text);
	}

	.form {
		display: grid;
		gap: var(--sp-3);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-2);
	}

	.result {
		display: flex;
		align-items: flex-start;
		gap: var(--sp-2);
		font-size: var(--fs-small);
	}

	.result :global(svg) {
		flex: none;
		margin-top: 2px;
	}

	.result.ok {
		color: var(--success);
	}

	.result.error {
		color: var(--danger);
	}

	.result.warn {
		color: var(--text-muted);
	}

	.help {
		border-radius: var(--r-lg);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.help summary {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		padding: var(--sp-3) var(--sp-4);
		cursor: pointer;
		list-style: none;
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
	}

	.help summary::-webkit-details-marker {
		display: none;
	}

	.help summary :global(svg) {
		color: var(--accent-ink);
	}

	.chev {
		display: inline-flex;
		margin-left: auto;
		transition: transform var(--dur-fast) var(--ease);
	}

	.help[open] .chev {
		transform: rotate(180deg);
	}

	.help ol {
		display: grid;
		gap: var(--sp-2);
		margin: 0;
		padding: 0 var(--sp-4) 0 calc(var(--sp-4) + 18px);
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.help a {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--accent-ink);
		font-weight: var(--fw-bold);
	}

	.note {
		padding: var(--sp-3) var(--sp-4) var(--sp-4);
		color: var(--text-muted);
		font-size: var(--fs-micro);
	}
</style>
