<script lang="ts">
	// Player side panel: audio tracks, subtitles (+ size), quality, playback information toggle.
	// Choices are radio groups (arrow keys move and select, as native radios do).
	import type { PlaybackTrack, SubtitleTrack } from '$lib/api/types';
	import { QUALITY_PRESETS, type QualityChoice } from '$lib/player/capabilities';
	import { audioTrackLabel, subtitleTrackLabel } from '$lib/player/labels';
	import type { SubtitleSize } from '$lib/player/prefs';
	import Icon from '../Icon.svelte';

	interface Props {
		audioTracks: PlaybackTrack[];
		subtitleTracks: SubtitleTrack[];
		selectedAudio: number;
		selectedSubtitle: number;
		quality: QualityChoice;
		/** Height actually played, for "Auto · 1080p". */
		currentHeight?: number;
		subtitleSize: SubtitleSize;
		stats: boolean;
		isAudio?: boolean;
		/** Section to scroll to and focus when opening. */
		focus?: 'subtitles' | null;
		onaudio: (index: number) => void;
		onsubtitle: (index: number) => void;
		onquality: (q: QualityChoice) => void;
		onsubtitlesize: (s: SubtitleSize) => void;
		onstats: (on: boolean) => void;
		onclose: () => void;
	}

	let {
		audioTracks,
		subtitleTracks,
		selectedAudio,
		selectedSubtitle,
		quality,
		currentHeight,
		subtitleSize,
		stats,
		isAudio = false,
		focus = null,
		onaudio,
		onsubtitle,
		onquality,
		onsubtitlesize,
		onstats,
		onclose
	}: Props = $props();

	let panel = $state<HTMLElement>();

	$effect(() => {
		if (!panel) return;
		const target =
			(focus === 'subtitles' && panel.querySelector<HTMLElement>('#pp-subs [aria-checked="true"]')) ||
			panel.querySelector<HTMLElement>('[aria-checked="true"]') ||
			panel.querySelector<HTMLElement>('button');
		target?.focus({ preventScroll: focus !== 'subtitles' });
		if (focus === 'subtitles') panel.querySelector('#pp-subs')?.scrollIntoView({ block: 'start' });
	});

	const qualities = Object.entries(QUALITY_PRESETS) as [QualityChoice, (typeof QUALITY_PRESETS)[QualityChoice]][];
	const sizes: { id: SubtitleSize; label: string; name: string }[] = [
		{ id: 's', label: 'S', name: 'Petite' },
		{ id: 'm', label: 'M', name: 'Moyenne' },
		{ id: 'l', label: 'L', name: 'Grande' }
	];

	/** Radio group keyboard: arrows move the selection, like native radios. */
	function radioKeys(e: KeyboardEvent) {
		const keys = ['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'];
		if (!keys.includes(e.key)) return;
		const group = e.currentTarget as HTMLElement;
		const radios = [...group.querySelectorAll<HTMLElement>('[role="radio"]')];
		const i = radios.indexOf(document.activeElement as HTMLElement);
		if (i < 0) return;
		e.preventDefault();
		e.stopPropagation();
		const next = radios[(i + (e.key === 'ArrowDown' || e.key === 'ArrowRight' ? 1 : -1) + radios.length) % radios.length];
		next.focus();
		next.click();
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.stopPropagation();
			onclose();
		}
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<aside class="panel" bind:this={panel} aria-label="Réglages de lecture" {onkeydown}>
	<header>
		<h2>Réglages</h2>
		<button type="button" class="close" aria-label="Fermer les réglages" onclick={onclose}><Icon name="x" size={18} /></button>
	</header>

	{#if audioTracks.length > 1 || (audioTracks.length && !isAudio)}
		<section aria-labelledby="pp-audio-t">
			<h3 id="pp-audio-t"><Icon name="audio" size={16} />Audio</h3>
			<!-- svelte-ignore a11y_interactive_supports_focus -->
			<div class="options" role="radiogroup" aria-labelledby="pp-audio-t" onkeydown={radioKeys}>
				{#each audioTracks as t (t.index)}
					{@const l = audioTrackLabel(t)}
					{@const on = t.index === selectedAudio}
					<button type="button" role="radio" aria-checked={on} tabindex={on ? 0 : -1} onclick={() => !on && onaudio(t.index)}>
						<span class="mark" aria-hidden="true">{#if on}<Icon name="check" size={16} />{/if}</span>
						<span class="text"><span class="main">{l.main}</span>{#if l.detail}<span class="detail">{l.detail}</span>{/if}</span>
					</button>
				{/each}
			</div>
		</section>
	{/if}

	{#if !isAudio}
		<section id="pp-subs" aria-labelledby="pp-subs-t">
			<h3 id="pp-subs-t"><Icon name="captions" size={16} />Sous-titres</h3>
			<!-- svelte-ignore a11y_interactive_supports_focus -->
			<div class="options" role="radiogroup" aria-labelledby="pp-subs-t" onkeydown={radioKeys}>
				<button type="button" role="radio" aria-checked={selectedSubtitle < 0} tabindex={selectedSubtitle < 0 ? 0 : -1} onclick={() => selectedSubtitle >= 0 && onsubtitle(-1)}>
					<span class="mark" aria-hidden="true">{#if selectedSubtitle < 0}<Icon name="check" size={16} />{/if}</span>
					<span class="text"><span class="main">Aucun</span></span>
				</button>
				{#each subtitleTracks as t (t.index)}
					{@const l = subtitleTrackLabel(t)}
					{@const on = t.index === selectedSubtitle}
					<button type="button" role="radio" aria-checked={on} tabindex={on ? 0 : -1} onclick={() => !on && onsubtitle(t.index)}>
						<span class="mark" aria-hidden="true">{#if on}<Icon name="check" size={16} />{/if}</span>
						<span class="text">
							<span class="main">{l.main}</span>
							<span class="detail">{l.detail}{t.delivery === 'burn' ? ' · incrustés (transcodage)' : ''}</span>
						</span>
					</button>
				{/each}
			</div>

			{#if subtitleTracks.length}
				<div class="size">
					<span id="pp-size-t" class="size-label">Taille</span>
					<!-- svelte-ignore a11y_interactive_supports_focus -->
					<div class="segmented" role="radiogroup" aria-labelledby="pp-size-t" onkeydown={radioKeys}>
						{#each sizes as s (s.id)}
							{@const on = s.id === subtitleSize}
							<button type="button" role="radio" aria-checked={on} aria-label={s.name} tabindex={on ? 0 : -1} onclick={() => onsubtitlesize(s.id)}>{s.label}</button>
						{/each}
					</div>
				</div>
			{/if}
		</section>

		<section aria-labelledby="pp-q-t">
			<h3 id="pp-q-t"><Icon name="gauge" size={16} />Qualité</h3>
			<!-- svelte-ignore a11y_interactive_supports_focus -->
			<div class="options" role="radiogroup" aria-labelledby="pp-q-t" onkeydown={radioKeys}>
				{#each qualities as [id, q] (id)}
					{@const on = id === quality}
					<button type="button" role="radio" aria-checked={on} tabindex={on ? 0 : -1} onclick={() => !on && onquality(id)}>
						<span class="mark" aria-hidden="true">{#if on}<Icon name="check" size={16} />{/if}</span>
						<span class="text">
							<span class="main">{q.label}{id === 'auto' && on && currentHeight ? ` · ${currentHeight}p` : ''}</span>
							{#if q.maxBitrate}<span class="detail">jusqu’à {(q.maxBitrate / 1_000_000).toLocaleString('fr-FR')} Mb/s</span>{:else}<span class="detail">selon l’écran et le fichier</span>{/if}
						</span>
					</button>
				{/each}
			</div>
		</section>
	{/if}

	<section class="switch-row">
		<span id="pp-stats-t">Informations de lecture</span>
		<button type="button" class="switch" role="switch" aria-checked={stats} aria-labelledby="pp-stats-t" onclick={() => onstats(!stats)}>
			<span class="knob" aria-hidden="true"></span>
		</button>
	</section>
</aside>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: var(--sp-5);
		width: min(360px, 100%);
		height: 100%;
		padding: calc(var(--sp-5) + env(safe-area-inset-top, 0px)) calc(var(--sp-5) + env(safe-area-inset-right, 0px)) calc(var(--sp-5) + env(safe-area-inset-bottom, 0px)) var(--sp-5);
		overflow-y: auto;
		overscroll-behavior: contain;
		background: var(--tint), rgba(7, 23, 37, 0.94);
		box-shadow: var(--shadow-3), inset 1px 0 0 var(--line);
		color: var(--text);
		scrollbar-width: thin;
	}

	@supports (backdrop-filter: blur(1px)) {
		.panel {
			background: var(--tint), rgba(7, 23, 37, 0.84);
			backdrop-filter: blur(18px) saturate(1.1);
		}
	}

	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	h2 {
		font-size: var(--fs-h2);
		line-height: var(--lh-h2);
		font-weight: var(--fw-bold);
	}

	.close {
		display: grid;
		place-items: center;
		width: 36px;
		height: 36px;
		border-radius: var(--r-sm);
		color: var(--text-muted);
	}

	.close:hover {
		color: var(--text);
		background: rgba(246, 255, 255, 0.07);
	}

	h3 {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		margin-bottom: var(--sp-2);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.options {
		display: grid;
		gap: 2px;
	}

	[role='radio']:not(.segmented [role='radio']) {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		width: 100%;
		min-height: 44px;
		padding: 6px var(--sp-3) 6px var(--sp-2);
		border-radius: var(--r-sm);
		text-align: left;
		transition: background-color var(--dur-fast) var(--ease);
	}

	[role='radio']:not(.segmented [role='radio']):hover {
		background: rgba(246, 255, 255, 0.05);
	}

	[role='radio'][aria-checked='true']:not(.segmented [role='radio']) {
		background: var(--surface-active);
	}

	.mark {
		display: grid;
		place-items: center;
		flex: none;
		width: 20px;
		color: var(--accent);
	}

	.text {
		display: grid;
		min-width: 0;
	}

	.main {
		font-weight: var(--fw-bold);
		overflow-wrap: anywhere;
	}

	.detail {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.size {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-top: var(--sp-3);
		padding-left: var(--sp-2);
	}

	.size-label {
		font-weight: var(--fw-medium);
		color: var(--text-muted);
	}

	.segmented {
		display: inline-flex;
		padding: 3px;
		border-radius: var(--r-full);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
	}

	.segmented [role='radio'] {
		min-width: 40px;
		height: 30px;
		border-radius: var(--r-full);
		font-weight: var(--fw-bold);
		color: var(--text-muted);
	}

	.segmented [role='radio'][aria-checked='true'] {
		background: var(--surface-active);
		color: var(--text);
	}

	.switch-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
		margin-top: auto;
		padding-top: var(--sp-4);
		box-shadow: inset 0 1px 0 var(--line);
		font-weight: var(--fw-bold);
	}

	.switch {
		position: relative;
		flex: none;
		width: 44px;
		height: 26px;
		border-radius: var(--r-full);
		background: rgba(246, 255, 255, 0.16);
		transition: background-color var(--dur-fast) var(--ease);
	}

	.switch[aria-checked='true'] {
		background: var(--accent);
	}

	.knob {
		position: absolute;
		top: 3px;
		left: 3px;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--text);
		transition: transform var(--dur-fast) var(--ease);
	}

	.switch[aria-checked='true'] .knob {
		transform: translateX(18px);
		background: var(--accent-contrast);
	}

	@media (prefers-reduced-motion: reduce) {
		.knob {
			transition: none;
		}
	}
</style>
