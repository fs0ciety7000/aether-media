<script lang="ts">
	// "Comment installer l'agent ?" — short, expandable (native <details>: keyboard and screen readers for free).
	import Icon from '../Icon.svelte';

	interface Props {
		open?: boolean;
	}

	let { open = false }: Props = $props();

	const toml = `listen = "0.0.0.0:8097"
# tls_cert = "/etc/aether/cert.pem"
# tls_key  = "/etc/aether/key.pem"
[[share]]
name = "films"
path = "/srv/media/films"
[[share]]
name = "series"
path = "/srv/media/series"`;
</script>

<details class="help" {open}>
	<summary><Icon name="info" size={16} />Comment installer l’agent ?<span class="chev" aria-hidden="true"><Icon name="chevron-down" size={16} /></span></summary>
	<div class="content">
		<ol>
			<li>
				Sur la machine qui possède les médias, copiez le binaire <code>aether</code> et créez un fichier <code>agent.toml</code> listant les
				dossiers à partager :
				<pre><code>{toml}</code></pre>
			</li>
			<li>
				Lancez l’agent :
				<pre><code>aether agent --config agent.toml</code></pre>
			</li>
			<li>
				Au premier démarrage, l’agent affiche son <strong>jeton d’appairage</strong> dans les journaux et l’enregistre dans
				<code>&lt;data&gt;/agent.token</code>. Collez-le ci-dessus avec l’adresse <code>http://hote:8097</code>.
			</li>
		</ol>
		<p class="note">
			Sans TLS (<code>tls_cert</code>, <code>tls_key</code>), n’exposez l’agent qu’à travers un VPN (WireGuard, Tailscale) ou un proxy HTTPS.
		</p>
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
