<script lang="ts">
	import { theme, THEMES, type ThemePref } from '$lib/theme.svelte';
	import Icon from './Icon.svelte';

	let open = $state(false);
	let root = $state<HTMLElement>();
	let trigger = $state<HTMLButtonElement>();

	const options: { id: ThemePref; label: string; swatch: string[] }[] = [
		...THEMES.map((t) => ({ id: t.id as ThemePref, label: t.label, swatch: [t.swatch.bg, t.swatch.surface, t.swatch.accent] })),
		{ id: 'system', label: 'Système (Nuit ↔ Lin)', swatch: ['#04121D', '#F3EFE8', '#5EC4C0'] }
	];

	function onWindowPointer(e: PointerEvent) {
		if (open && root && !root.contains(e.target as Node)) open = false;
	}

	function onKeydown(e: KeyboardEvent) {
		if (open && e.key === 'Escape') {
			open = false;
			trigger?.focus();
		}
	}
</script>

<svelte:window onpointerdown={onWindowPointer} onkeydown={onKeydown} />

<div class="switcher" bind:this={root}>
	<button
		bind:this={trigger}
		type="button"
		class="trigger"
		aria-expanded={open}
		aria-controls="theme-panel"
		aria-label="Apparence"
		onclick={() => (open = !open)}
	>
		<Icon name="contrast" />
	</button>

	{#if open}
		<div class="panel" id="theme-panel">
			<fieldset>
				<legend class="micro">Thème</legend>
				{#each options as opt (opt.id)}
					<label class="option" class:checked={theme.pref === opt.id}>
						<input type="radio" name="theme" value={opt.id} checked={theme.pref === opt.id} onchange={() => theme.set(opt.id)} />
						<span class="swatch" aria-hidden="true">
							{#each opt.swatch as c, i (i)}<span style:background={c}></span>{/each}
						</span>
						<span class="name">{opt.label}</span>
						<span class="tick" aria-hidden="true"><Icon name="check" size={16} /></span>
					</label>
				{/each}
			</fieldset>
			<label class="toggle">
				<span>Grain pellicule</span>
				<input type="checkbox" role="switch" checked={theme.grain} onchange={(e) => theme.setGrain(e.currentTarget.checked)} />
			</label>
		</div>
	{/if}
</div>

<style>
	.switcher {
		position: relative;
	}

	.trigger {
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		border-radius: var(--r-md);
		background: var(--surface);
		box-shadow: inset 0 0 0 1px var(--line);
		color: var(--text-muted);
		transition:
			color var(--dur-fast) var(--ease),
			background-color var(--dur-fast) var(--ease),
			box-shadow var(--dur-fast) var(--ease);
	}

	.trigger:hover,
	.trigger[aria-expanded='true'] {
		color: var(--text);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.panel {
		position: absolute;
		right: 0;
		top: calc(100% + var(--sp-2));
		z-index: 40;
		width: 264px;
		padding: var(--sp-2);
		background: var(--tint), var(--surface-raised);
		border-radius: var(--r-lg);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
		color: var(--text);
		animation: appear var(--dur-base) var(--ease);
	}

	@keyframes appear {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}

	fieldset {
		border: 0;
		padding: 0;
		margin: 0;
	}

	legend {
		padding: var(--sp-2) var(--sp-3) var(--sp-2);
		color: var(--text-muted);
	}

	.option {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		height: 40px;
		padding: 0 var(--sp-3);
		border-radius: var(--r-sm);
		font-size: var(--fs-body);
		color: var(--text-muted);
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.option:hover {
		background: color-mix(in srgb, var(--text) 6%, transparent);
		color: var(--text);
	}

	.option.checked {
		color: var(--text);
	}

	.option input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.option:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: -2px;
	}

	.swatch {
		display: flex;
		width: 30px;
		height: 18px;
		border-radius: var(--r-xs);
		overflow: hidden;
		box-shadow: 0 0 0 1px var(--line);
	}

	.swatch span {
		flex: 1;
	}

	.swatch span:last-child {
		flex: 0 0 8px;
	}

	.name {
		flex: 1;
	}

	.tick {
		display: flex;
		color: var(--accent-ink);
		opacity: 0;
	}

	.checked .tick {
		opacity: 1;
	}

	.toggle {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
		padding: var(--sp-3) var(--sp-3) var(--sp-2);
		border-top: 1px solid var(--line);
		font-size: var(--fs-body);
		color: var(--text-muted);
		cursor: pointer;
	}

	.toggle input {
		appearance: none;
		position: relative;
		width: 34px;
		height: 20px;
		border-radius: var(--r-full);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		cursor: pointer;
		transition: background-color var(--dur-base) var(--ease);
	}

	.toggle input::after {
		content: '';
		position: absolute;
		top: 3px;
		left: 3px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--text-muted);
		transition:
			transform var(--dur-base) var(--ease),
			background-color var(--dur-base) var(--ease);
	}

	.toggle input:checked {
		background: var(--accent);
	}

	.toggle input:checked::after {
		transform: translateX(14px);
		background: var(--accent-contrast);
	}
</style>
