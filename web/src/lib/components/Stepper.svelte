<script lang="ts" module>
	export interface Step {
		label: string;
		/** Skipped steps (e.g. no code for an agent already paired) read as done. */
		skipped?: boolean;
		onclick?: () => void;
	}
</script>

<script lang="ts">
	// Progress through a guided flow: numbered steps, done ones get a check, the current one is marked
	// with aria-current="step". Purely informative (navigation stays with the flow's own buttons),
	// except steps given an `onclick`, which become buttons (go back to a finished step).
	import Icon from './Icon.svelte';

	interface Props {
		steps: Step[];
		/** Index of the current step. */
		current: number;
		/** Accessible name of the list. */
		label?: string;
	}

	let { steps, current, label = 'Étapes' }: Props = $props();
</script>

<ol class="stepper" aria-label={label}>
	{#each steps as step, i (i)}
		{@const state = i < current || step.skipped ? 'done' : i === current ? 'current' : 'todo'}
		<li class={['step', state]} aria-current={i === current ? 'step' : undefined}>
			{#snippet body()}
				<span class="dot" aria-hidden="true">
					{#if state === 'done'}<Icon name="check" size={14} />{:else}{i + 1}{/if}
				</span>
				<span class="lbl">{step.label}</span>
				<span class="visually-hidden">
					{state === 'done' ? (step.skipped ? '(inutile)' : '(terminée)') : state === 'current' ? '(en cours)' : '(à venir)'}
				</span>
			{/snippet}
			{#if step.onclick && i < current}
				<button type="button" class="hit" onclick={step.onclick}>{@render body()}</button>
			{:else}
				<span class="hit">{@render body()}</span>
			{/if}
		</li>
	{/each}
</ol>

<style>
	.stepper {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.step {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		min-width: 0;
		flex: 1 1 0;
	}

	/* Connector between two steps. */
	.step + .step::before {
		content: '';
		flex: 0 1 28px;
		min-width: 8px;
		height: 1.5px;
		border-radius: 1px;
		background: var(--line-strong);
	}

	.step.done + .step::before,
	.step.done + .step.current::before {
		background: color-mix(in srgb, var(--accent) 55%, transparent);
	}

	.hit {
		display: inline-flex;
		align-items: center;
		gap: var(--sp-2);
		min-width: 0;
		padding: 2px 4px 2px 2px;
		border-radius: var(--r-full);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	button.hit {
		cursor: pointer;
		transition: color var(--dur-fast) var(--ease);
	}

	button.hit:hover {
		color: var(--text);
	}

	.dot {
		display: grid;
		place-items: center;
		flex: none;
		width: 26px;
		height: 26px;
		border-radius: 50%;
		font-size: var(--fs-micro);
		font-variant-numeric: tabular-nums;
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1.5px var(--line-strong);
		color: var(--text-muted);
	}

	.done .dot {
		background: var(--accent-soft);
		box-shadow: none;
		color: var(--accent-ink);
	}

	.current .dot {
		background: var(--accent);
		box-shadow: none;
		color: var(--accent-contrast);
	}

	.current .hit {
		color: var(--text);
	}

	.lbl {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Phones: only the current step keeps its label. */
	@media (max-width: 767px) {
		.step {
			flex: 0 1 auto;
		}

		.step.current {
			flex: 1 1 auto;
		}

		.step:not(.current) .lbl {
			position: absolute;
			width: 1px;
			height: 1px;
			overflow: hidden;
			clip: rect(0 0 0 0);
			white-space: nowrap;
		}
	}
</style>
