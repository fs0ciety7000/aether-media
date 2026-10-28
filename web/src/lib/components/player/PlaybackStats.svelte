<script lang="ts">
	// "Informations de lecture": discreet stats-for-nerds overlay.
	import type { PlaybackInfo } from '$lib/api/types';
	import { formatBitrate } from '$lib/format';
	import { codecName } from '$lib/media';
	import type { EngineStats } from '$lib/player/engine';
	import { METHOD_LABEL, reasonLabel } from '$lib/player/labels';

	interface Props {
		info: PlaybackInfo;
		engine?: string;
		stats: EngineStats;
		/** Seconds buffered ahead of the playhead. */
		bufferAhead: number;
		/** Rendered video size (decoder output). */
		videoSize?: { width: number; height: number };
		fileBitrate?: number;
		audioLabel?: string;
	}

	let { info, engine, stats, bufferAhead, videoSize, fileBitrate, audioLabel }: Props = $props();

	const ENGINE_LABEL: Record<string, string> = { direct: 'Fichier (progressif)', 'native-hls': 'HLS natif', 'hls.js': 'hls.js (MSE)' };
	const v = $derived(info.video);
	const res = $derived(v ? `${v.width}×${v.height}` : videoSize?.width ? `${videoSize.width}×${videoSize.height}` : undefined);
</script>

<section class="stats" aria-label="Informations de lecture">
	<dl>
		<div>
			<dt>Méthode</dt>
			<dd>
				<strong>{METHOD_LABEL[info.method]}</strong>
				{#if info.reasons.length}<span class="muted"> — {info.reasons.map(reasonLabel).join(', ')}</span>{/if}
			</dd>
		</div>
		{#if v || res}
			<div>
				<dt>Vidéo</dt>
				<dd>
					{v ? codecName(v.codec) : '—'}{res ? ` · ${res}` : ''}{v?.hdr ? ` · ${v.hdr}` : ''}
					{#if v && info.method !== 'direct'}<span class="muted"> · {v.copied ? 'copiée' : 'transcodée'}</span>{/if}
				</dd>
			</div>
		{/if}
		{#if audioLabel}<div><dt>Audio</dt><dd>{audioLabel}</dd></div>{/if}
		<div>
			<dt>Débit</dt>
			<dd>
				{formatBitrate(stats.levelBitrate ?? fileBitrate) ?? '—'}
				{#if stats.bandwidth}<span class="muted"> · réseau ≈ {formatBitrate(stats.bandwidth)}</span>{/if}
			</dd>
		</div>
		<div><dt>Tampon</dt><dd class="tabular">{bufferAhead.toLocaleString('fr-FR', { maximumFractionDigits: 1 })} s</dd></div>
		{#if stats.totalFrames}
			<div><dt>Images perdues</dt><dd class="tabular">{stats.droppedFrames ?? 0} / {stats.totalFrames}</dd></div>
		{/if}
		<div><dt>Moteur</dt><dd>{(engine && ENGINE_LABEL[engine]) ?? engine ?? '—'} · session {info.sessionId.slice(0, 8)}</dd></div>
	</dl>
</section>

<style>
	.stats {
		max-width: min(420px, calc(100vw - 32px));
		padding: var(--sp-3) var(--sp-4);
		border-radius: var(--r-md);
		background: rgba(3, 13, 22, 0.78);
		box-shadow: inset 0 0 0 1px var(--line);
		font-size: var(--fs-micro);
		line-height: 1.5;
		color: var(--text);
		pointer-events: auto;
	}

	dl {
		display: grid;
		gap: 2px;
	}

	dl div {
		display: grid;
		grid-template-columns: 7.5em minmax(0, 1fr);
		gap: var(--sp-2);
	}

	dt {
		color: var(--text-muted);
		font-weight: var(--fw-medium);
	}

	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}

	.muted {
		color: var(--text-muted);
	}
</style>
