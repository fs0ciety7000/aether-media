<script lang="ts">
	// Where a setting comes from and when a change applies: a quiet line under the field.
	// Read-only values (configuration file, environment) say how to change them instead.
	import type { SettingSource } from '$lib/api/types';
	import { SOURCE_LABEL } from '$lib/admin';
	import Icon from '../Icon.svelte';

	interface Props {
		source: SettingSource;
		readOnly?: boolean;
		applies?: 'live' | 'restart';
		restartRequired?: boolean;
		/** Environment variable that overrides the setting. */
		env?: string;
		id?: string;
	}

	let { source, readOnly = false, applies = 'live', restartRequired = false, env, id }: Props = $props();
</script>

<p class="meta" {id}>
	{#if readOnly}
		<span class="item lock"><Icon name="lock" size={13} />Défini par la configuration</span>
		<span class="item">{SOURCE_LABEL[source]}{#if env && source === 'env'}{' · '}<code>{env}</code>{/if}</span>
	{:else}
		<span class="item">{SOURCE_LABEL[source]}</span>
		{#if restartRequired}
			<span class="item warn"><Icon name="refresh" size={13} />Enregistré · effectif au redémarrage</span>
		{:else if applies === 'restart'}
			<span class="item"><Icon name="refresh" size={13} />Effectif au redémarrage</span>
		{:else}
			<span class="item"><Icon name="check" size={13} />Appliqué immédiatement</span>
		{/if}
	{/if}
</p>

<style>
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: 4px var(--sp-3);
		color: var(--text-muted);
		font-size: var(--fs-micro);
		line-height: 1.4;
	}

	.item {
		display: inline-flex;
		align-items: center;
		gap: 5px;
	}

	.lock {
		color: var(--accent-ink);
		font-weight: var(--fw-bold);
	}

	.warn {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.warn :global(svg) {
		color: var(--accent);
	}

	code {
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		color: var(--text);
	}
</style>
