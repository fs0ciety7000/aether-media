<script lang="ts">
	// Technical details from ffprobe: container, video, audio tracks, subtitles.
	import type { MediaFile } from '$lib/api/types';
	import { formatBitrate, formatBytes, formatTrackTime, metaLine } from '$lib/format';
	import { channelsLabel, codecName, languageName, resolutionLabel } from '$lib/media';
	import SectionHeader from '../SectionHeader.svelte';

	interface Props {
		files: MediaFile[];
	}

	let { files }: Props = $props();
	const uid = $props.id();
</script>

{#if files.length}
	<section class="tech" aria-labelledby="tech-{uid}">
		<SectionHeader title="Informations techniques" id="tech-{uid}" />
		{#each files as f (f.id)}
			{@const video = f.streams.filter((s) => s.type === 'video')}
			{@const audio = f.streams.filter((s) => s.type === 'audio')}
			{@const subs = f.streams.filter((s) => s.type === 'subtitle')}
			<div class="file panel">
				<div class="file-head">
					<p class="path" title={f.path}>{f.path.split('/').pop()}</p>
					<p class="file-meta tabular">
						{metaLine(f.part ? `Partie ${f.part}` : undefined, f.container?.toUpperCase(), formatBytes(f.size), formatBitrate(f.bitrate), formatTrackTime(f.durationMs))}
					</p>
				</div>
				{#if !f.probed}
					<p class="note">{f.probeError ? `Analyse impossible : ${f.probeError}` : 'Analyse du fichier en attente.'}</p>
				{:else}
					<div class="cols">
						<div class="col">
							<h3 class="micro">Vidéo</h3>
							{#each video as v (v.index)}
								<p class="line">
									<strong>{metaLine(resolutionLabel(v.width, v.height), v.hdr)}</strong>
									<span class="tabular">{metaLine(codecName(v.codec), v.profile, v.width && v.height ? `${v.width}×${v.height}` : undefined, v.bitDepth ? `${v.bitDepth} bits` : undefined, v.frameRate ? `${Math.round(v.frameRate * 100) / 100} i/s` : undefined)}</span>
								</p>
							{:else}
								<p class="line muted">Aucune piste vidéo</p>
							{/each}
						</div>
						<div class="col">
							<h3 class="micro">Audio · {audio.length}</h3>
							{#each audio as a (a.index)}
								<p class="line">
									<strong>{languageName(a.language) ?? 'Langue inconnue'}{a.default ? ' · par défaut' : ''}</strong>
									<span>{metaLine(a.title, codecName(a.codec), channelsLabel(a))}</span>
								</p>
							{:else}
								<p class="line muted">Aucune piste audio</p>
							{/each}
						</div>
						<div class="col">
							<h3 class="micro">Sous-titres · {subs.length}</h3>
							{#each subs as s (s.index)}
								<p class="line">
									<strong>{languageName(s.language) ?? 'Langue inconnue'}</strong>
									<span>{metaLine(s.title, codecName(s.codec), s.forced ? 'forcés' : undefined, s.default ? 'par défaut' : undefined)}</span>
								</p>
							{:else}
								<p class="line muted">Aucun</p>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/each}
	</section>
{/if}

<style>
	.tech {
		display: grid;
		gap: var(--gap);
		min-width: 0;
	}

	.tech :global(.section-header) {
		margin-bottom: 0;
	}

	.file {
		padding: var(--sp-4) var(--sp-5) var(--sp-5);
	}

	.file-head {
		display: grid;
		gap: 2px;
		padding-bottom: var(--sp-4);
		margin-bottom: var(--sp-4);
		box-shadow: 0 1px 0 var(--line);
	}

	.path {
		font-weight: var(--fw-bold);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.file-meta,
	.note {
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.cols {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--sp-5);
	}

	h3 {
		margin-bottom: var(--sp-2);
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.line {
		display: grid;
		gap: 1px;
		padding: 6px 0;
		font-size: var(--fs-small);
		color: var(--text-muted);
	}

	.line + .line {
		box-shadow: 0 -1px 0 var(--line);
	}

	.line strong {
		color: var(--text);
		font-weight: var(--fw-bold);
	}

	.muted {
		color: var(--text-faint);
	}

	@media (max-width: 767px) {
		.cols {
			grid-template-columns: minmax(0, 1fr);
			gap: var(--sp-3);
		}

		.file {
			padding: var(--sp-4);
		}
	}
</style>
