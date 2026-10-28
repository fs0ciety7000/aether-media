<script lang="ts">
	// On/off setting: a native checkbox with role="switch", label and optional hint.
	interface Props {
		checked?: boolean;
		label: string;
		hint?: string;
		disabled?: boolean;
		onchange?: (checked: boolean) => void;
	}

	let { checked = $bindable(false), label, hint, disabled = false, onchange }: Props = $props();
	const uid = $props.id();
</script>

<label class="switch" class:disabled>
	<span class="text">
		<span class="label">{label}</span>
		{#if hint}<span class="hint" id="sw-{uid}-hint">{hint}</span>{/if}
	</span>
	<input
		type="checkbox"
		role="switch"
		bind:checked
		{disabled}
		aria-describedby={hint ? `sw-${uid}-hint` : undefined}
		onchange={() => onchange?.(checked)}
	/>
</label>

<style>
	.switch {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-4);
		cursor: pointer;
	}

	.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.text {
		display: grid;
		gap: 2px;
		min-width: 0;
	}

	.label {
		font-size: var(--fs-body);
		font-weight: var(--fw-medium);
		color: var(--text);
	}

	.hint {
		font-size: var(--fs-micro);
		line-height: var(--lh-micro);
		color: var(--text-muted);
	}

	input {
		appearance: none;
		position: relative;
		flex: none;
		width: 38px;
		height: 22px;
		border-radius: var(--r-full);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		cursor: inherit;
		transition: background-color var(--dur-base) var(--ease);
	}

	input::after {
		content: '';
		position: absolute;
		top: 3px;
		left: 3px;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--text-muted);
		transition:
			transform var(--dur-base) var(--ease),
			background-color var(--dur-base) var(--ease);
	}

	input:checked {
		background: var(--accent);
		box-shadow: none;
	}

	input:checked::after {
		transform: translateX(16px);
		background: var(--accent-contrast);
	}

	input:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
</style>
