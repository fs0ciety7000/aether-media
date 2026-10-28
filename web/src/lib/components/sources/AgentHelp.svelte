<script lang="ts">
	// "Comment installer l'agent ?" — short, expandable (native <details>: keyboard and screen readers for free).
	// M5c: the agent runs without any configuration file; pairing is done by code from this interface.
	import Icon from '../Icon.svelte';

	interface Props {
		open?: boolean;
		/** Manual token form (agent.toml): explain the legacy flow instead. */
		legacy?: boolean;
	}

	let { open = false, legacy = false }: Props = $props();

	const toml = `listen = "0.0.0.0:8097"
[[share]]
name = "films"
path = "/srv/media/films"`;
</script>

<details class="help" {open}>
	<summary><Icon name="info" size={16} />Comment installer l’agent ?<span class="chev" aria-hidden="true"><Icon name="chevron-down" size={16} /></span></summary>
	<div class="content">
		{#if legacy}
			<ol>
				<li>
					Sur la machine qui possède les médias, déclarez les dossiers à partager dans <code>agent.toml</code> :
					<pre><code>{toml}</code></pre>
				</li>
				<li>Lancez <code>aether agent --config agent.toml</code>.</li>
				<li>
					Le <strong>jeton</strong> est affiché dans les journaux au premier démarrage et enregistré dans
					<code>&lt;data&gt;/agent.token</code>. Collez-le ci-dessus avec l’adresse <code>https://hote:8097</code>.
				</li>
			</ol>
			<p class="note">Plus simple : lancez <code>aether agent</code> sans configuration et utilisez l’appairage guidé par code.</p>
		{:else}
			<ol>
				<li>
					Sur la machine qui possède les médias (NAS, PC du salon…), copiez le binaire <code>aether</code> et lancez simplement :
					<pre><code>aether agent</code></pre>
					Aucun fichier de configuration : l’agent écoute sur le port 8097 et se signale sur le réseau local.
				</li>
				<li>
					L’agent affiche un <strong>code d’appairage</strong> (<code>XXXX-XXXX</code>, renouvelé toutes les 10 minutes) dans ses
					journaux et sur sa page d’état <code>https://localhost:8097</code>, à ouvrir sur la machine de l’agent.
				</li>
				<li>
					Ici, cliquez sur « Rechercher sur le réseau » (ou saisissez son adresse), tapez le code, puis choisissez les dossiers à partager.
				</li>
			</ol>
			<p class="note">
				En Docker, la recherche demande <code>network_mode: host</code> (port UDP 7360) ; sinon, saisissez l’adresse. Sans TLS, n’exposez
				l’agent qu’à travers un réseau local, un VPN (WireGuard, Tailscale) ou un proxy HTTPS. <code>agent.toml</code> reste possible pour
				les réglages avancés (<code>browse_roots</code>, TLS, partages fixes).
			</p>
		{/if}
	</div>
</details>

<style>
	.help {
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
		display: inline-grid;
		margin-left: auto;
		color: var(--text-muted);
		transition: transform var(--dur-fast) var(--ease);
	}

	.help[open] .chev {
		transform: rotate(180deg);
	}

	.content {
		display: grid;
		gap: var(--sp-3);
		padding: 0 var(--sp-4) var(--sp-4);
	}

	ol {
		display: grid;
		gap: var(--sp-3);
		padding-left: 1.2em;
	}

	pre {
		margin-top: var(--sp-2);
		padding: var(--sp-3);
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		overflow-x: auto;
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.78rem;
		color: var(--text);
	}

	pre code {
		white-space: pre;
	}

	.note {
		font-size: var(--fs-micro);
	}
</style>
