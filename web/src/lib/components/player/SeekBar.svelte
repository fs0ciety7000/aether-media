<script lang="ts">
	// Thin progress bar that thickens on hover: buffered ranges, hover time tooltip, drag to scrub
	// (the seek is committed on release — HLS segments are produced on demand, so no live seeking).
	// role="slider" with a French aria-valuetext; ←/→ ±10 s, PageUp/PageDown ±1 min, Home/End.
	import { formatClock } from '$lib/format';

	interface Props {
		/** Seconds (absolute, file timeline). */
		current: number;
		duration: number;
		buffered?: [number, number][];
		/** Chapter starts in seconds (ticks); none when unknown. */
		chapters?: number[];
		onseek: (seconds: number) => void;
		/** Scrubbing started / ended (keeps the controls visible). */
		onscrub?: (active: boolean) => void;
	}

	let { current, duration, buffered = [], chapters = [], onseek, onscrub }: Props = $props();

	let track = $state<HTMLElement>();
	let hoverX = $state<number | null>(null);
	let scrubTime = $state<number | null>(null);
	let width = $state(0);

	const safeDuration = $derived(duration > 0 && Number.isFinite(duration) ? duration : 0);
	const shown = $derived(scrubTime ?? current);
	const ratio = (t: number) => (safeDuration ? Math.min(1, Math.max(0, t / safeDuration)) : 0);
	const hoverTime = $derived(hoverX === null || !safeDuration ? null : ratio((hoverX / Math.max(1, width)) * safeDuration) * safeDuration);
	const tipTime = $derived(scrubTime ?? hoverTime);
	const tipLeft = $derived(tipTime === null ? 0 : Math.min(Math.max(ratio(tipTime) * width, 28), Math.max(28, width - 28)));
	const valueText = $derived(`${formatClock(shown)} sur ${formatClock(safeDuration)}`);

	function timeAt(clientX: number): number {
		if (!track || !safeDuration) return 0;
		const r = track.getBoundingClientRect();
		return ratio(((clientX - r.left) / r.width) * safeDuration) * safeDuration;
	}

	function onpointerdown(e: PointerEvent) {
		if (e.button !== 0 || !safeDuration) return;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		scrubTime = timeAt(e.clientX);
		onscrub?.(true);
		e.preventDefault();
		(e.currentTarget as HTMLElement).focus({ preventScroll: true });
	}

	function onpointermove(e: PointerEvent) {
		if (!track) return;
		const r = track.getBoundingClientRect();
		if (e.pointerType !== 'touch') hoverX = Math.min(Math.max(0, e.clientX - r.left), r.width);
		if (scrubTime !== null) scrubTime = timeAt(e.clientX);
	}

	function finish(commit: boolean) {
		if (scrubTime === null) return;
		const t = scrubTime;
		scrubTime = null;
		onscrub?.(false);
		if (commit) onseek(t);
	}

	function onkeydown(e: KeyboardEvent) {
		if (!safeDuration) return;
		const step: Record<string, number> = { ArrowLeft: -10, ArrowRight: 10, ArrowDown: -10, ArrowUp: 10, PageDown: -60, PageUp: 60 };
		let t: number | undefined;
		if (e.key in step) t = current + step[e.key];
		else if (e.key === 'Home') t = 0;
		else if (e.key === 'End') t = Math.max(0, safeDuration - 1);
		if (t === undefined) return;
		e.preventDefault();
		e.stopPropagation();
		onseek(Math.min(Math.max(0, t), safeDuration));
	}
</script>

<div
	class="seek"
	class:scrubbing={scrubTime !== null}
	role="slider"
	tabindex="0"
	aria-label="Position de lecture"
	aria-valuemin={0}
	aria-valuemax={Math.round(safeDuration)}
	aria-valuenow={Math.round(shown)}
	aria-valuetext={valueText}
	{onpointerdown}
	{onpointermove}
	onpointerup={() => finish(true)}
	onpointercancel={() => finish(false)}
	onpointerleave={() => (hoverX = null)}
	{onkeydown}
>
	<div class="track" bind:this={track} bind:clientWidth={width}>
		{#each buffered as [s, e], i (i)}
			<span class="buffered" style:left="{ratio(s) * 100}%" style:width="{(ratio(e) - ratio(s)) * 100}%"></span>
		{/each}
		{#if hoverTime !== null && scrubTime === null}
			<span class="hover" style:width="{ratio(hoverTime) * 100}%"></span>
		{/if}
		<span class="played" style:width="{ratio(shown) * 100}%"></span>
		{#each chapters as c (c)}
			{#if c > 0 && c < safeDuration}<span class="tick" style:left="{ratio(c) * 100}%"></span>{/if}
		{/each}
		<span class="thumb" style:left="{ratio(shown) * 100}%"></span>
	</div>
	{#if tipTime !== null}
		<span class="tip tabular" style:left="{tipLeft}px" aria-hidden="true">{formatClock(tipTime)}</span>
	{/if}
</div>

<style>
	.seek {
		position: relative;
		display: flex;
		align-items: center;
		height: 24px;
		cursor: pointer;
		touch-action: none;
		border-radius: var(--r-xs);
		outline-offset: 2px;
	}

	.track {
		position: relative;
		width: 100%;
		height: 4px;
		border-radius: var(--r-full);
		background: rgba(246, 255, 255, 0.18);
		transition: height var(--dur-fast) var(--ease);
	}

	.seek:hover .track,
	.seek:focus-visible .track,
	.scrubbing .track {
		height: 8px;
	}

	.buffered,
	.played,
	.hover {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		border-radius: inherit;
	}

	.buffered {
		background: rgba(246, 255, 255, 0.26);
	}

	.hover {
		background: rgba(246, 255, 255, 0.16);
	}

	.played {
		background: var(--accent);
	}

	.tick {
		position: absolute;
		top: 0;
		bottom: 0;
		width: 2px;
		margin-left: -1px;
		background: rgba(2, 8, 13, 0.7);
	}

	.thumb {
		position: absolute;
		top: 50%;
		width: 14px;
		height: 14px;
		margin: -7px 0 0 -7px;
		border-radius: 50%;
		background: var(--text);
		box-shadow: 0 1px 4px rgba(0, 0, 0, 0.4);
		transform: scale(0);
		transition: transform var(--dur-fast) var(--ease);
	}

	.seek:hover .thumb,
	.seek:focus-visible .thumb,
	.scrubbing .thumb {
		transform: scale(1);
	}

	.tip {
		position: absolute;
		bottom: calc(100% + 6px);
		transform: translateX(-50%);
		padding: 3px 8px;
		border-radius: var(--r-xs);
		background: rgba(4, 14, 22, 0.88);
		box-shadow: inset 0 0 0 1px var(--line-strong);
		color: var(--text);
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		pointer-events: none;
		white-space: nowrap;
	}

	@media (pointer: coarse) {
		.seek {
			height: 32px;
		}

		.thumb {
			transform: scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.track,
		.thumb {
			transition: none;
		}
	}
</style>
