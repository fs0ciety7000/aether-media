<script lang="ts">
	// Inline theme choice (profile page): the same options as the top-bar switcher, as large swatches.
	import { theme, THEMES, type ThemePref } from '$lib/theme.svelte';
	import Icon from './Icon.svelte';
	import Switch from './Switch.svelte';

	const options: { id: ThemePref; label: string; mood: string; swatch: string[] }[] = [
		...THEMES.map((t) => ({ id: t.id as ThemePref, label: t.label, mood: t.mood, swatch: [t.swatch.bg, t.swatch.surface, t.swatch.accent] })),
		{ id: 'system', label: 'Système', mood: 'Nuit ↔ Lin', swatch: ['#04121D', '#F3EFE8', '#5EC4C0'] }
	];
	let grain = $state(theme.grain);
</script>

<fieldset class="picker">
	<legend class="visually-hidden">Thème</legend>
	<div class="grid">
		{#each options as opt (opt.id)}
			<label class="opt" class:checked={theme.pref === opt.id}>
				<input type="radio" name="theme-inline" value={opt.id} checked={theme.pref === opt.id} onchange={() => theme.set(opt.id)} />
				<span class="swatch" aria-hidden="true">
					{#each opt.swatch as c, i (i)}<span style:background={c}></span>{/each}
				</span>
				<span class="text">
					<span class="name">{opt.label}</span>
					<span class="mood">{opt.mood}</span>
				</span>
				<span class="tick" aria-hidden="true"><Icon name="check" size={16} /></span>
			</label>
		{/each}
	</div>
</fieldset>
<div class="grain">
	<Switch label="Grain pellicule" hint="Une texture discrète sur toute l’interface." bind:checked={grain} onchange={(on) => theme.setGrain(on)} />
</div>

<style>
	.picker {
		border: 0;
		margin: 0;
		padding: 0;
		min-width: 0;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
		gap: var(--sp-3);
	}

	.opt {
		position: relative;
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		padding: var(--sp-3);
		border-radius: var(--r-md);
		background: var(--surface-raised);
		box-shadow: inset 0 0 0 1px var(--line);
		cursor: pointer;
		transition: background-color var(--dur-fast) var(--ease);
	}

	.opt:hover {
		background: color-mix(in srgb, var(--surface-raised) 88%, var(--text));
	}

	.opt.checked {
		background: var(--surface-active);
		box-shadow: inset 0 0 0 1.5px color-mix(in srgb, var(--accent) 60%, transparent);
	}

	.opt:has(input:focus-visible) {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}

	.swatch {
		display: grid;
		grid-template-columns: repeat(3, 12px);
		flex: none;
		height: 34px;
		border-radius: var(--r-sm);
		overflow: hidden;
		box-shadow: inset 0 0 0 1px var(--line-strong);
	}

	.text {
		display: grid;
		min-width: 0;
		line-height: 1.3;
	}

	.name {
		font-weight: var(--fw-bold);
	}

	.mood {
		font-size: var(--fs-micro);
		color: var(--text-muted);
	}

	.tick {
		margin-left: auto;
		color: var(--accent);
		opacity: 0;
	}

	.checked .tick {
		opacity: 1;
	}

	.grain {
		margin-top: var(--sp-4);
		max-width: 520px;
	}
</style>
