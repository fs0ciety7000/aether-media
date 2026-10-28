<script lang="ts">
	// One playback in progress (Réglages › Lectures): who, what, how (direct / remux / transcode and
	// why), bitrate, progress; the administrator can stop it.
	import type { ActiveSession } from '$lib/api/types';
	import { hwLabel, METHOD_LABEL, reasonLabel } from '$lib/admin';
	import { formatBitrate, formatClock } from '$lib/format';
	import { codecName, hueFrom, resolutionLabel } from '$lib/media';
	import Artwork from '../Artwork.svelte';
	import Avatar from '../Avatar.svelte';
	import Button from '../Button.svelte';
	import Icon from '../Icon.svelte';
	import ProgressLine from '../ProgressLine.svelte';
	import Tag from '../Tag.svelte';

	interface Props {
		session: ActiveSession;
		onstop?: (s: ActiveSession) => void;
		busy?: boolean;
	}

	let { session: s, onstop, busy = false }: Props = $props();

	const it = $derived(s.item);
	const title = $derived(it ? (it.kind === 'episode' && it.seriesTitle ? it.seriesTitle : it.title) : 'Élément inconnu');
	const subtitle = $derived(
		it?.kind === 'episode'
			? `S${String(it.season ?? 0).padStart(2, '0')}E${String(it.episode ?? 0).padStart(2, '0')} · ${it.title}`
			: it?.year
				? String(it.year)
				: undefined
	);
	const square = $derived(it?.kind === 'track' || it?.kind === 'album');
	const wide = $derived(it?.kind === 'episode');
	const imgType = $derived(wide ? 'thumb' : 'poster');
	const progress = $derived(s.durationMs > 0 ? s.positionMs / s.durationMs : 0);
	const video = $derived(s.video);
	const videoLine = $derived(
		video
			? `${codecName(video.codec)}${resolutionLabel(video.width, video.height) ? ' ' + resolutionLabel(video.width, video.height) : ''} · ${video.copied ? 'copie' : 'réencodé'}`
			: undefined
	);
	const audioLine = $derived(s.audio ? `${codecName(s.audio.codec)} · ${s.audio.copied ? 'copie' : 'réencodé'}` : undefined);
</script>

<article class="session panel" aria-label="{s.user?.name ?? 'Utilisateur'} regarde {title}">
	<div class={['art', { wide, square }]}>
		{#if it}
			<Artwork
				src="/api/v1/items/{it.id}/images/{imgType}?w=185"
				ratio={wide ? '16 / 9' : square ? '1 / 1' : '2 / 3'}
				sizes="92px"
				placeholder={{ title: it.title, icon: wide ? 'tv' : 'film' }}
			/>
		{/if}
	</div>
	<div class="body">
		<header class="top">
			<div class="who">
				<Avatar name={s.user?.name ?? '?'} size={28} hue={hueFrom(s.user?.name ?? '?')} />
				<span>{s.user?.name ?? 'Utilisateur inconnu'}</span>
				{#if s.paused}<Tag icon="pause">En pause</Tag>{/if}
			</div>
			<Tag variant={s.method === 'direct' ? 'surface' : 'accent'} icon={s.method === 'transcode' ? 'cpu' : s.method === 'remux' ? 'layers' : 'play'}>
				{METHOD_LABEL[s.method] ?? s.method}
			</Tag>
		</header>
		<p class="title">
			{#if it}<a href="/item/{it.id}">{title}</a>{:else}{title}{/if}
		</p>
		{#if subtitle}<p class="sub">{subtitle}</p>{/if}
		{#if s.reasons.length}
			<p class="reasons"><Icon name="info" size={14} /><span>{s.reasons.map(reasonLabel).join(' · ')}</span></p>
		{/if}
		<dl class="facts">
			{#if s.bitrate}<div><dt>Débit</dt><dd class="tabular">{formatBitrate(s.bitrate)}</dd></div>{/if}
			{#if videoLine}<div><dt>Vidéo</dt><dd>{videoLine}</dd></div>{/if}
			{#if audioLine}<div><dt>Audio</dt><dd>{audioLine}</dd></div>{/if}
			{#if s.hwAccel}<div><dt>Encodeur</dt><dd>{hwLabel(s.hwAccel)}</dd></div>{/if}
		</dl>
		<div class="progress">
			<ProgressLine value={progress} variant="inline" />
			<span class="time tabular">{formatClock(s.positionMs / 1000)} / {formatClock(s.durationMs / 1000)}</span>
		</div>
	</div>
	<div class="actions">
		<Button variant="tertiary" icon="stop" disabled={busy} onclick={() => onstop?.(s)} aria-label="Arrêter la lecture de {s.user?.name ?? 'cet utilisateur'}">Arrêter</Button>
	</div>
</article>

<style>
	.session {
		display: grid;
		grid-template-columns: 92px minmax(0, 1fr) auto;
		gap: var(--sp-4);
		align-items: start;
		padding: var(--sp-4);
		border-radius: var(--r-lg);
	}

	.art {
		border-radius: var(--r-md);
		overflow: hidden;
	}

	.body {
		display: grid;
		gap: var(--sp-1);
		min-width: 0;
	}

	.top {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-2);
	}

	.who {
		display: flex;
		align-items: center;
		gap: var(--sp-2);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
	}

	.title {
		margin-top: var(--sp-1);
		font-size: var(--fs-body);
		font-weight: var(--fw-bold);
		overflow-wrap: anywhere;
	}

	.title a:hover {
		color: var(--accent-ink);
	}

	.sub {
		color: var(--text-muted);
		font-size: var(--fs-small);
	}

	.reasons {
		display: flex;
		align-items: flex-start;
		gap: 6px;
		color: var(--text-muted);
		font-size: var(--fs-micro);
	}

	.reasons :global(svg) {
		flex: none;
		margin-top: 1px;
		color: var(--accent-ink);
	}

	.facts {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sp-1) var(--sp-5);
		margin-top: var(--sp-2);
	}

	.facts div {
		display: grid;
		gap: 0;
	}

	dt {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	dd {
		font-size: var(--fs-small);
	}

	.progress {
		display: flex;
		align-items: center;
		gap: var(--sp-3);
		margin-top: var(--sp-2);
	}

	.progress :global(.progress) {
		flex: 1;
	}

	.time {
		color: var(--text-muted);
		font-size: var(--fs-micro);
		white-space: nowrap;
	}

	@media (max-width: 767px) {
		.session {
			grid-template-columns: 64px minmax(0, 1fr);
			gap: var(--sp-3);
		}

		.actions {
			grid-column: 1 / -1;
		}

		.actions :global(.btn) {
			width: 100%;
		}
	}
</style>
