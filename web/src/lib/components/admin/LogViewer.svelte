<script lang="ts">
	// Monospace journal (Réglages › Journaux): one line per record, level as a quiet tag, attributes
	// muted. Follows the end while `follow` is on, unless the reader scrolled up.
	import { tick } from 'svelte';
	import type { LogEntry } from '$lib/api/types';
	import { LOG_LEVEL_LABEL } from '$lib/admin';

	interface Props {
		entries: LogEntry[];
		follow?: boolean;
		/** Text to highlight (search). */
		highlight?: string;
		label?: string;
	}

	let { entries, follow = false, highlight = '', label = 'Journal du serveur' }: Props = $props();

	let box = $state<HTMLElement>();
	let pinned = true;

	function time(iso: string) {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	}

	function day(iso: string) {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'short' });
	}

	function onScroll() {
		if (!box) return;
		pinned = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
	}

	$effect(() => {
		void entries.length;
		if (!follow || !pinned) return;
		tick().then(() => box?.scrollTo({ top: box.scrollHeight }));
	});

	/** Splits text around the (case-insensitive) highlight. */
	function parts(text: string): { t: string; hit: boolean }[] {
		const q = highlight.trim();
		if (!q) return [{ t: text, hit: false }];
		const out: { t: string; hit: boolean }[] = [];
		const lower = text.toLowerCase();
		const ql = q.toLowerCase();
		let i = 0;
		for (;;) {
			const j = lower.indexOf(ql, i);
			if (j < 0) break;
			if (j > i) out.push({ t: text.slice(i, j), hit: false });
			out.push({ t: text.slice(j, j + q.length), hit: true });
			i = j + q.length;
		}
		if (i < text.length) out.push({ t: text.slice(i), hit: false });
		return out;
	}
</script>

{#snippet hl(text: string)}{#each parts(text) as p, i (i)}{#if p.hit}<mark>{p.t}</mark>{:else}{p.t}{/if}{/each}{/snippet}

<!-- A scrollable region must be focusable to be scrolled with the keyboard. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="viewer" bind:this={box} onscroll={onScroll} role="log" aria-label={label} aria-live="off" tabindex="0">
	{#if entries.length}
		<ol>
			{#each entries as e (e.seq)}
				<li class={['line', e.level.toLowerCase()]}>
					<time datetime={e.time} title="{day(e.time)} {time(e.time)}">{time(e.time)}</time>
					<span class="lvl">{LOG_LEVEL_LABEL[e.level] ?? e.level}</span>
					<span class="msg">{@render hl(e.msg)}{#each e.attrs ?? [] as a, i (i)}<span class="attr">{' '}<span class="k">{a.k}</span>=<span class="v">{@render hl(a.v)}</span></span>{/each}</span>
				</li>
			{/each}
		</ol>
	{:else}
		<p class="empty">Aucune entrée pour ces filtres.</p>
	{/if}
</div>

<style>
	.viewer {
		max-height: min(64vh, 680px);
		min-height: 160px;
		overflow: auto;
		padding: var(--sp-2) 0;
		border-radius: var(--r-lg);
		background: var(--bg-sunken);
		box-shadow: inset 0 0 0 1px var(--line);
		font-family: ui-monospace, 'SFMono-Regular', Menlo, monospace;
		font-size: 0.75rem;
		line-height: 1.55;
	}

	ol {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.line {
		display: grid;
		grid-template-columns: auto auto minmax(0, 1fr);
		gap: var(--sp-3);
		align-items: baseline;
		padding: 2px var(--sp-4);
	}

	.line:hover {
		background: color-mix(in srgb, var(--text) 4%, transparent);
	}

	time {
		color: var(--text-muted);
		font-variant-numeric: tabular-nums;
	}

	.lvl {
		min-width: 6.5ch;
		color: var(--text-muted);
		font-weight: 700;
	}

	.info .lvl {
		color: var(--accent-ink);
	}

	.warn .lvl {
		color: var(--text);
	}

	.error .lvl,
	.error .msg {
		color: var(--danger);
	}

	.debug .msg {
		color: var(--text-muted);
	}

	.msg {
		color: var(--text);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}

	.attr {
		color: var(--text-muted);
	}

	.k {
		color: var(--text-muted);
		opacity: 0.85;
	}

	mark {
		border-radius: 3px;
		background: var(--accent-soft);
		color: var(--accent-ink);
		font-weight: 700;
	}

	.empty {
		padding: var(--sp-5);
		color: var(--text-muted);
		font-family: var(--font-sans, inherit);
		font-size: var(--fs-small);
	}

	@media (max-width: 767px) {
		.line {
			grid-template-columns: auto minmax(0, 1fr);
			gap: 0 var(--sp-2);
			padding: 4px var(--sp-3);
		}

		.msg {
			grid-column: 1 / -1;
		}
	}
</style>
