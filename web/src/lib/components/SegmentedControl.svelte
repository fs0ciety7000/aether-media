<script lang="ts">
	// One choice among a few (log level, list filter): native radios styled as a pill group.
	// Arrow keys move between options (browser behaviour of a radio group).
	interface Option {
		value: string;
		label: string;
		/** Small counter after the label. */
		count?: number;
	}

	interface Props {
		options: Option[];
		value?: string;
		/** Accessible name of the group. */
		label: string;
		size?: 'sm' | 'md';
		onchange?: (value: string) => void;
	}

	let { options, value = $bindable(''), label, size = 'md', onchange }: Props = $props();
	const uid = $props.id();
</script>

<div class={['segmented', size]} role="radiogroup" aria-label={label}>
	{#each options as o (o.value)}
		<label class:checked={value === o.value}>
			<input type="radio" name="seg-{uid}" value={o.value} bind:group={value} onchange={() => onchange?.(o.value)} />
			<span>{o.label}</span>
			{#if o.count !== undefined}<span class="count tabular">{o.count.toLocaleString('fr-FR')}</span>{/if}
		</label>
	{/each}
</div>

<style>
	.segmented {
		display: inline-flex;
		gap: 4px;
		padding: 4px;
		border-radius: var(--r-full);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		max-width: 100%;
		overflow-x: auto;
		scrollbar-width: none;
	}

	label {
		position: relative;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 32px;
		padding: 0 14px;
		border-radius: var(--r-full);
		color: var(--text-muted);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		white-space: nowrap;
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.sm label {
		height: 28px;
		padding: 0 11px;
		font-size: var(--fs-micro);
	}

	label:hover {
		color: var(--text);
	}

	label.checked {
		background: var(--surface-active);
		color: var(--text);
	}

	input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	label:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 1px;
	}

	.count {
		min-width: 20px;
		padding: 1px 6px;
		border-radius: var(--r-full);
		background: color-mix(in srgb, var(--text) 8%, transparent);
		color: var(--text-muted);
		font-size: var(--fs-micro);
		text-align: center;
	}

	.checked .count {
		background: var(--accent-soft);
		color: var(--accent-ink);
	}
</style>
