<script lang="ts">
	// Full-screen player (video and simple audio). Owns the playback session:
	// POST /playback/{id} → attach (direct / native HLS / hls.js) → ping every 10 s → DELETE.
	// Changing audio track, burnt subtitles or quality restarts the session at the current
	// position; the last frame stays on screen meanwhile (canvas snapshot).
	import { onDestroy, onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, ApiError } from '$lib/api/client';
	import { firstImage, imageUrl } from '$lib/api/images';
	import type { Item, ItemDetail, PlaybackInfo } from '$lib/api/types';
	import { formatClock } from '$lib/format';
	import { episodeCode } from '$lib/media';
	import { degradeProfile, detectProfile, QUALITY_PRESETS, type QualityChoice } from '$lib/player/capabilities';
	import { attachSource, type Engine, type EngineStats } from '$lib/player/engine';
	import { audioTrackLabel, playbackErrorMessage, PlayerError, subtitleTrackLabel } from '$lib/player/labels';
	import { prefs, type SubtitleSize } from '$lib/player/prefs';
	import { resumeMs as resumePosition } from '$lib/player/resume';
	import { pickAudio, pickSubtitle } from '$lib/player/tracks';
	import { session } from '$lib/session.svelte';
	import { nextSibling } from '$lib/player/target';
	import Artwork from '../Artwork.svelte';
	import Button from '../Button.svelte';
	import Icon, { type IconName } from '../Icon.svelte';
	import PlaybackStats from './PlaybackStats.svelte';
	import PlayerPanel from './PlayerPanel.svelte';
	import SeekBar from './SeekBar.svelte';

	interface Props {
		itemId: number;
		/** ?t= — explicit start (seconds): no resume prompt. */
		startSeconds?: number;
		/** ?audio= / ?sub= — initial stream indexes. */
		audioIndex?: number;
		subtitleIndex?: number;
		onback: (item: ItemDetail | null) => void;
	}

	let { itemId, startSeconds, audioIndex, subtitleIndex, onback }: Props = $props();

	const PING_MS = 10_000;
	const HIDE_MS = 3000;

	// ——— Item ———
	let item = $state<ItemDetail | null>(null);
	/** Series (episodes) or album (tracks), for artwork and labels. */
	let parentArt = $state<ItemDetail | null>(null);
	let nextItem = $state<Item | null>(null);
	const isAudio = $derived(item?.kind === 'track');

	// ——— Session ———
	type Phase = 'loading' | 'prompt' | 'starting' | 'playing' | 'error';
	let phase = $state<Phase>('loading');
	let errorText = $state('');
	let info = $state<PlaybackInfo | null>(null);
	let engine: Engine | null = null;
	let engineKind = $state<string>();
	let sessionId: string | null = null;
	let mediaFileId: number | undefined;
	let token = 0;
	let destroyed = false;
	let profileLevel = 0;
	let resumeMs = $state(0);

	// ——— Media state (seconds, file timeline) ———
	let video = $state<HTMLVideoElement>();
	let root = $state<HTMLElement>();
	let snapshot = $state<HTMLCanvasElement>();
	/** Media timeline starts at `offset` when a server playlist begins at startMs (see onMeta). */
	let offset = 0;
	let current = $state(0);
	let mediaDuration = $state(0);
	let buffered = $state<[number, number][]>([]);
	let paused = $state(true);
	let waiting = $state(false);
	let ended = $state(false);
	let firstFrame = $state(false);
	let restarting = $state(false);
	let needsGesture = $state(false);
	let volume = $state(prefs.volume());
	let muted = $state(prefs.muted());
	let fullscreen = $state(false);
	let recovering = $state(false);

	// ——— Choices ———
	let quality = $state<QualityChoice>(prefs.quality());
	let subSize = $state<SubtitleSize>(prefs.subtitleSize());
	let showStats = $state(prefs.stats());
	let selectedAudio = $state(-1);
	/** Text subtitle shown through <track> (-1 none); burnt ones come from the session. */
	let clientSub = $state(-1);

	const burnedSub = $derived.by(() => {
		if (!info || info.selectedSubtitle < 0) return -1;
		return info.subtitleTracks.find((t) => t.index === info!.selectedSubtitle)?.delivery === 'burn' ? info.selectedSubtitle : -1;
	});
	const activeSub = $derived(burnedSub >= 0 ? burnedSub : clientSub);
	const vttTracks = $derived(info?.subtitleTracks.filter((t) => t.delivery === 'vtt' && t.url) ?? []);

	const duration = $derived(info?.durationMs ? info.durationMs / 1000 : mediaDuration ? mediaDuration + offset : (item?.runtimeMs ?? 0) / 1000);

	// ——— UI ———
	let panelOpen = $state(false);
	let panelFocus = $state<'subtitles' | null>(null);
	let controlsShown = $state(true);
	let hoveringControls = $state(false);
	let scrubbing = $state(false);
	let hideTimer: ReturnType<typeof setTimeout> | undefined;
	let osd = $state<{ icon?: IconName; text?: string; side?: 'left' | 'right'; key: number } | null>(null);
	let osdTimer: ReturnType<typeof setTimeout> | undefined;
	let stats = $state<EngineStats>({});
	let bufferAhead = $state(0);

	const forceVisible = $derived(paused || panelOpen || scrubbing || hoveringControls || ended || isAudio || phase !== 'playing');
	const controlsVisible = $derived(controlsShown || forceVisible);
	const spinning = $derived(phase === 'loading' || phase === 'starting' || restarting || recovering || (waiting && !paused && phase === 'playing'));
	const showNext = $derived(!!nextItem && item?.kind === 'episode' && phase === 'playing' && duration > 0 && (ended || duration - current < 40));

	// ——— Labels ———
	const seriesTitle = $derived(item?.kind === 'episode' ? item.ancestors[0]?.title : undefined);
	const artistTitle = $derived(item?.kind === 'track' ? item.ancestors.find((a) => a.kind === 'artist')?.title : undefined);
	const albumTitle = $derived(item?.kind === 'track' ? item.ancestors.find((a) => a.kind === 'album')?.title : undefined);
	const heading = $derived.by(() => {
		if (!item) return '';
		if (item.kind === 'episode') {
			const code = episodeCode(item);
			return code ? `${code} — ${item.title}` : item.title;
		}
		return item.title;
	});
	const eyebrow = $derived(seriesTitle ?? (artistTitle ? [artistTitle, albumTitle].filter(Boolean).join(' · ') : item?.year ? String(item.year) : undefined));
	const backdrop = $derived(
		isAudio
			? (firstImage(item, 'poster', 'thumb') ?? firstImage(parentArt, 'poster'))
			: (firstImage(item, 'backdrop', 'thumb') ?? firstImage(parentArt, 'backdrop', 'thumb'))
	);
	const cover = $derived(isAudio ? (firstImage(item, 'poster', 'thumb') ?? firstImage(parentArt, 'poster')) : undefined);
	const audioTrack = $derived(info?.audioTracks.find((t) => t.index === selectedAudio));
	const fileBitrate = $derived(item?.mediaFiles.find((f) => f.id === info?.mediaFileId)?.bitrate);
	const nextLabel = $derived(nextItem ? [episodeCode(nextItem), nextItem.title].filter(Boolean).join(' — ') : '');

	// ——— Loading ———
	async function loadItem() {
		try {
			const d = await api.item(itemId);
			if (destroyed) return;
			item = d;
			setMediaSession(d);
			const parentId = d.kind === 'episode' ? d.ancestors[0]?.id : d.kind === 'track' ? d.parentId : undefined;
			if (parentId) api.item(parentId).then((p) => (parentArt = p), () => {});
			if (d.kind === 'episode' || d.kind === 'track') nextSibling(d).then((n) => (nextItem = n), () => {});
		} catch (err) {
			if (!destroyed) fail(playbackErrorMessage(err));
			return;
		}
		if (startSeconds !== undefined && startSeconds >= 0) return startSession(startSeconds * 1000, initialOptions());
		// Server-side progress (M6): the position stored by the last session's pings.
		const r = item ? resumePosition(item) : 0;
		if (r && item?.kind !== 'track') {
			resumeMs = r;
			phase = 'prompt';
		} else startSession(0, initialOptions());
	}

	function chooseResume(resume: boolean) {
		startSession(resume ? resumeMs : 0, initialOptions());
	}

	// ——— Default tracks from the user's preferences ———
	/** Explicit ?audio / ?sub win; otherwise the preferences apply (once, to the first session). */
	// The route keys the player on the item: these props never change for an instance.
	const explicitTracks = untrack(() => audioIndex !== undefined || subtitleIndex !== undefined);
	let prefsPending = !explicitTracks;

	/** First request: tracks guessed from the probed streams of the first file. */
	function initialOptions(): StartOptions {
		if (explicitTracks || !item) return { audioIndex, burnIndex: subtitleIndex };
		const streams = item.mediaFiles.find((f) => f.streams.length)?.streams ?? [];
		const audios = streams.filter((st) => st.type === 'audio');
		const subs = streams.filter((st) => st.type === 'subtitle');
		const prefs = session.user?.prefs;
		const audio = pickAudio(audios, prefs);
		const playedAudio = audio ?? audios.find((a) => a.default) ?? audios[0];
		const sub = isAudioKind(item) ? -1 : pickSubtitle(subs, prefs, playedAudio?.language);
		return { audioIndex: audio?.index, burnIndex: sub >= 0 ? sub : undefined };
	}

	const isAudioKind = (d: ItemDetail) => d.kind === 'track';

	// ——— Session lifecycle ———
	interface StartOptions {
		audioIndex?: number;
		/** Subtitle stream to request from the server (burnt when it has to be). */
		burnIndex?: number;
		/** Keep the paused state across a restart. */
		autoplay?: boolean;
	}

	async function startSession(startMs: number, opts: StartOptions = {}) {
		const my = ++token;
		const restart = !!info && phase === 'playing';
		if (restart) takeSnapshot();
		restarting = restart;
		if (!restart) phase = 'starting';
		recovering = false;
		ended = false;
		needsGesture = false;
		current = startMs / 1000;
		teardown(false);

		const profile = degradeProfile(detectProfile(quality), profileLevel);
		let next: PlaybackInfo;
		try {
			next = await api.startPlayback(itemId, {
				profile,
				mediaFileId,
				audioIndex: opts.audioIndex !== undefined && opts.audioIndex >= 0 ? opts.audioIndex : undefined,
				subtitleIndex: opts.burnIndex !== undefined && opts.burnIndex >= 0 ? opts.burnIndex : undefined,
				startMs: Math.max(0, Math.round(startMs))
			});
		} catch (err) {
			if (my === token && !destroyed) fail(playbackErrorMessage(err));
			return;
		}
		if (my !== token || destroyed) {
			api.stopPlayback(next.sessionId);
			return;
		}

		// Preferences checked against the session's real tracks (external subtitles, other file…).
		if (prefsPending) {
			prefsPending = false;
			if (!isAudio) {
				const prefs = session.user?.prefs;
				const audio = pickAudio(next.audioTracks, prefs);
				const playedAudio = audio ?? next.audioTracks.find((t) => t.index === next.selectedAudio);
				const sub = pickSubtitle(next.subtitleTracks, prefs, playedAudio?.language);
				const subTrack = next.subtitleTracks.find((t) => t.index === sub);
				const audioChange = !!audio && audio.index !== next.selectedAudio;
				const burnChange = subTrack?.delivery === 'burn' && next.selectedSubtitle !== sub;
				if (audioChange || burnChange) {
					api.stopPlayback(next.sessionId);
					return startSession(startMs, { ...opts, audioIndex: audio?.index ?? next.selectedAudio, burnIndex: sub >= 0 ? sub : undefined });
				}
				if (opts.burnIndex === undefined) opts = { ...opts, burnIndex: subTrack?.delivery === 'vtt' ? sub : undefined };
			}
		}

		info = next;
		sessionId = next.sessionId;
		mediaFileId = next.mediaFileId;
		offset = 0;
		mediaDuration = 0;
		if (!restart) firstFrame = false;
		selectedAudio = next.selectedAudio;
		const requested = opts.burnIndex ?? -1;
		const wanted = requested >= 0 ? requested : clientSub;
		const wantedTrack = next.subtitleTracks.find((t) => t.index === wanted);
		clientSub = wantedTrack?.delivery === 'vtt' ? wanted : -1;
		current = next.startMs / 1000;

		try {
			if (!video) throw new PlayerError('other', 'Lecteur indisponible.');
			engine = await attachSource(video, next, {
				hls: profile.hls,
				startSeconds: next.startMs / 1000,
				onFatal,
				onRecovering: () => (recovering = true)
			});
		} catch (err) {
			if (my === token) onFatal(err instanceof PlayerError ? err : new PlayerError('other', playbackErrorMessage(err)));
			return;
		}
		if (my !== token || destroyed) return;
		engineKind = engine.kind;
		phase = 'playing';
		startPing();
		if (opts.autoplay !== false) play();
	}

	function onFatal(err: PlayerError) {
		if (destroyed) return;
		// Decode or format failure: step down (no direct play, then H.264/AAC only) before giving up.
		if ((err.kind === 'media' || err.kind === 'unsupported') && profileLevel < 2) {
			profileLevel++;
			startSession(position() * 1000, { audioIndex: selectedAudio, burnIndex: burnedSub, autoplay: !paused || !firstFrame });
			return;
		}
		fail(err.message);
	}

	function fail(message: string) {
		token++;
		teardown(false);
		errorText = message;
		phase = 'error';
		restarting = false;
		recovering = false;
	}

	/**
	 * Stops the engine and the server session (keepalive when the page is going away).
	 * The DELETE carries the exact position: the server stores it and finalises the played state.
	 */
	function teardown(keepalive: boolean) {
		const sid = sessionId;
		const last = sid && firstFrame ? { positionMs: Math.round((ended ? duration : position()) * 1000), paused: true } : null;
		stopPing();
		engine?.destroy();
		engine = null;
		sessionId = null;
		if (!sid) return;
		api.stopPlayback(sid, keepalive, last?.positionMs);
	}

	function retry() {
		const at = position() * 1000 || (startSeconds ?? 0) * 1000;
		phase = 'starting';
		startSession(at, { audioIndex: selectedAudio >= 0 ? selectedAudio : audioIndex, burnIndex: burnedSub >= 0 ? burnedSub : undefined });
	}

	/** Keeps the last frame on screen while a new session starts. */
	function takeSnapshot() {
		if (!video || !snapshot || !video.videoWidth || isAudio) return;
		snapshot.width = video.videoWidth;
		snapshot.height = video.videoHeight;
		try {
			snapshot.getContext('2d')?.drawImage(video, 0, 0);
		} catch {
			/* tainted or not ready */
		}
	}

	// ——— Ping & resume ———
	let pingTimer: ReturnType<typeof setInterval> | undefined;

	function startPing() {
		stopPing();
		pingTimer = setInterval(() => {
			const sid = sessionId;
			if (!sid) return;
			api.pingPlayback(sid, { positionMs: Math.round(position() * 1000), paused }).catch((err) => {
				// Session reaped server-side: start a fresh one where we are (HLS needs it).
				if (err instanceof ApiError && err.status === 404 && sid === sessionId && info?.method !== 'direct') {
					startSession(position() * 1000, { audioIndex: selectedAudio, burnIndex: burnedSub, autoplay: !paused });
				}
			});
		}, PING_MS);
	}

	function stopPing() {
		clearInterval(pingTimer);
		pingTimer = undefined;
	}

	function position(): number {
		if (!engine || !video || restarting) return current;
		return (video.currentTime || 0) + offset;
	}

	/** Out-of-band ping (pause, tab hidden, next episode): the server keeps the position. */
	function saveNow() {
		if (!item || !sessionId || phase !== 'playing' || !firstFrame || restarting) return;
		api.pingPlayback(sessionId, { positionMs: Math.round((ended ? duration : position()) * 1000), paused }).catch(() => {});
	}

	// ——— Transport ———
	async function play() {
		if (!video) return;
		try {
			await video.play();
			needsGesture = false;
		} catch (err) {
			if ((err as DOMException)?.name === 'NotAllowedError') {
				needsGesture = true;
				restarting = false;
			}
		}
	}

	function togglePlay() {
		if (!video || phase !== 'playing') return;
		if (ended) {
			seek(0);
			play();
		} else if (video.paused) play();
		else video.pause();
		flash(video.paused ? 'pause' : 'play');
	}

	function seek(t: number) {
		if (phase !== 'playing' || !duration) return;
		const target = Math.min(Math.max(0, t), Math.max(0, duration - 0.5));
		const local = target - offset;
		ended = false;
		if (!engine || !video || local < 0) {
			// Before the start of this session's playlist: new session there.
			startSession(target * 1000, { audioIndex: selectedAudio, burnIndex: burnedSub, autoplay: !paused });
			return;
		}
		video.currentTime = local;
		current = target;
	}

	function seekBy(delta: number, side?: 'left' | 'right') {
		seek(position() + delta);
		flash(delta < 0 ? 'back-10' : 'forward-10', `${delta < 0 ? '−' : '+'}${Math.abs(delta)} s`, side);
	}

	function setVolume(v: number) {
		if (!video) return;
		volume = Math.min(1, Math.max(0, Math.round(v * 100) / 100));
		video.volume = volume;
		if (volume > 0 && video.muted) video.muted = false;
		prefs.setVolume(volume);
		flash(volume === 0 ? 'volume-x' : volume < 0.5 ? 'volume-low' : 'volume', `${Math.round(volume * 100)} %`);
	}

	function toggleMute() {
		if (!video) return;
		video.muted = !video.muted;
		if (!video.muted && video.volume === 0) video.volume = volume = 0.5;
		flash(video.muted ? 'volume-x' : 'volume', video.muted ? 'Son coupé' : `${Math.round(video.volume * 100)} %`);
	}

	const volumeIcon = $derived<IconName>(muted || volume === 0 ? 'volume-x' : volume < 0.5 ? 'volume-low' : 'volume');

	// ——— Fullscreen ———
	type FsElement = HTMLElement & { webkitRequestFullscreen?: () => void };
	type FsDocument = Document & { webkitFullscreenElement?: Element; webkitExitFullscreen?: () => void };
	type IosVideo = HTMLVideoElement & { webkitEnterFullscreen?: () => void; webkitSupportsFullscreen?: boolean };

	async function toggleFullscreen() {
		const doc = document as FsDocument;
		if (doc.fullscreenElement ?? doc.webkitFullscreenElement) {
			if (doc.exitFullscreen) await doc.exitFullscreen().catch(() => {});
			else doc.webkitExitFullscreen?.();
			return;
		}
		const el = root as FsElement | undefined;
		if (!el) return;
		if (el.requestFullscreen) {
			try {
				await el.requestFullscreen({ navigationUI: 'hide' });
			} catch {
				return;
			}
		} else if (el.webkitRequestFullscreen) el.webkitRequestFullscreen();
		else {
			// iPhone: only the video element itself can go full screen (native controls).
			(video as IosVideo | undefined)?.webkitEnterFullscreen?.();
			return;
		}
		lockLandscape();
	}

	function lockLandscape() {
		const o = screen.orientation as ScreenOrientation & { lock?: (o: string) => Promise<void> };
		if (isAudio || !matchMedia('(pointer: coarse)').matches || !o?.lock) return;
		if (video && video.videoWidth && video.videoWidth < video.videoHeight) return;
		o.lock('landscape').catch(() => {});
	}

	function onFullscreenChange() {
		const doc = document as FsDocument;
		fullscreen = !!(doc.fullscreenElement ?? doc.webkitFullscreenElement);
		if (!fullscreen) (screen.orientation as ScreenOrientation & { unlock?: () => void })?.unlock?.();
	}

	// ——— Choices ———
	function selectAudio(index: number) {
		if (!info || index === selectedAudio) return;
		selectedAudio = index;
		startSession(position() * 1000, { audioIndex: index, burnIndex: burnedSub, autoplay: !paused });
	}

	function selectSubtitle(index: number) {
		if (!info) return;
		const track = info.subtitleTracks.find((t) => t.index === index);
		if (index >= 0 && !track) return;
		if (track?.delivery === 'burn') {
			clientSub = -1;
			startSession(position() * 1000, { audioIndex: selectedAudio, burnIndex: index, autoplay: !paused });
		} else if (burnedSub >= 0) {
			// Leaving burnt subtitles: a clean session, then the text track client-side.
			clientSub = index;
			startSession(position() * 1000, { audioIndex: selectedAudio, autoplay: !paused });
		} else clientSub = index;
		const label = track ? subtitleTrackLabel(track).main : 'Aucun';
		flash('captions', `Sous-titres : ${label}`);
	}

	function cycleSubtitles() {
		if (!info || isAudio) return;
		const order = [-1, ...info.subtitleTracks.map((t) => t.index)];
		if (order.length < 2) return flash('captions', 'Aucun sous-titre');
		selectSubtitle(order[(order.indexOf(activeSub) + 1) % order.length]);
	}

	function selectQuality(q: QualityChoice) {
		quality = q;
		prefs.setQuality(q);
		if (!info) return;
		// Direct play that already fits the new limits keeps going; anything else restarts.
		const preset = QUALITY_PRESETS[q];
		const height = info.video?.height ?? 0;
		const fits =
			info.method === 'direct' && (!preset.maxHeight || height <= preset.maxHeight) && (!preset.maxBitrate || (!!fileBitrate && fileBitrate <= preset.maxBitrate));
		if (!fits) startSession(position() * 1000, { audioIndex: selectedAudio, burnIndex: burnedSub, autoplay: !paused });
	}

	function setSubSize(s: SubtitleSize) {
		subSize = s;
		prefs.setSubtitleSize(s);
	}

	function setStats(on: boolean) {
		showStats = on;
		prefs.setStats(on);
	}

	function openPanel(focus: 'subtitles' | null = null) {
		panelFocus = focus;
		panelOpen = true;
	}

	let panelButton = $state<HTMLButtonElement>();
	function closePanel() {
		panelOpen = false;
		panelButton?.focus();
	}

	// ——— Text tracks: show the selected one; shift cues when the timeline is offset ———
	$effect(() => {
		const want = clientSub;
		void vttTracks;
		if (!video) return;
		for (const el of video.querySelectorAll('track')) {
			el.track.mode = Number(el.dataset.index) === want ? 'showing' : 'disabled';
		}
	});

	function onTrackLoad(e: Event) {
		if (!offset) return;
		const cues = (e.currentTarget as HTMLTrackElement).track.cues;
		if (!cues) return;
		for (const cue of cues) {
			cue.startTime -= offset;
			cue.endTime -= offset;
		}
	}

	// ——— Media events ———
	function onMeta() {
		if (!video || !info) return;
		const d = video.duration;
		const full = info.durationMs / 1000;
		const tail = (info.durationMs - info.startMs) / 1000;
		// A playlist covering only [startMs, end] (instead of the whole file): shift the timeline.
		if (info.method !== 'direct' && info.startMs > 5000 && Number.isFinite(d) && Math.abs(d - tail) < 1.5 && Math.abs(d - full) > 5) {
			offset = info.startMs / 1000;
			video.currentTime = 0;
		}
		mediaDuration = Number.isFinite(d) ? d : 0;
	}

	function onTime() {
		if (!video || restarting || !engine) return;
		current = video.currentTime + offset;
		updateBuffered();
	}

	function updateBuffered() {
		if (!video) return;
		const out: [number, number][] = [];
		const b = video.buffered;
		let ahead = 0;
		for (let i = 0; i < b.length; i++) {
			out.push([b.start(i) + offset, b.end(i) + offset]);
			if (b.start(i) <= video.currentTime + 0.5 && b.end(i) > video.currentTime) ahead = b.end(i) - video.currentTime;
		}
		buffered = out;
		bufferAhead = ahead;
	}

	function onPlaying() {
		waiting = false;
		recovering = false;
		restarting = false;
		firstFrame = true;
		paused = false;
	}

	function onEnded() {
		ended = true;
		paused = true;
		saveNow();
		if (isAudio && nextItem) goNext();
	}

	function onVolumeChange() {
		if (!video) return;
		muted = video.muted;
		volume = video.volume;
		prefs.setMuted(video.muted);
	}

	function goNext() {
		if (!nextItem) return;
		saveNow();
		goto(`/lecture/${nextItem.id}`, { replaceState: true });
	}

	// ——— Controls visibility ———
	function poke() {
		controlsShown = true;
		clearTimeout(hideTimer);
		hideTimer = setTimeout(() => (controlsShown = false), HIDE_MS);
	}

	function flash(icon?: IconName, text?: string, side?: 'left' | 'right') {
		osd = { icon, text, side, key: (osd?.key ?? 0) + 1 };
		clearTimeout(osdTimer);
		osdTimer = setTimeout(() => (osd = null), 700);
	}

	// Mouse: click toggles play, double-click toggles full screen.
	// Touch: tap shows/hides the controls, double-tap on a side seeks ±10 s (repeat to add).
	let lastPointer = 'mouse';
	let lastTap = 0;
	let tapTimer: ReturnType<typeof setTimeout> | undefined;

	function onSurfaceDown(e: PointerEvent) {
		lastPointer = e.pointerType;
	}

	function onSurfaceClick(e: MouseEvent) {
		if (lastPointer !== 'mouse') return touchTap(e);
		if (needsGesture) return play();
		togglePlay();
		poke();
	}

	function touchTap(e: MouseEvent) {
		const rect = root?.getBoundingClientRect();
		const zone = rect ? (e.clientX - rect.left) / rect.width : 0.5;
		const now = performance.now();
		if (now - lastTap < 300 && phase === 'playing') {
			clearTimeout(tapTimer);
			lastTap = now;
			if (zone < 0.4) seekBy(-10, 'left');
			else if (zone > 0.6) seekBy(10, 'right');
			return;
		}
		lastTap = now;
		clearTimeout(tapTimer);
		tapTimer = setTimeout(() => {
			if (needsGesture) return play();
			if (controlsVisible && !forceVisible) {
				clearTimeout(hideTimer);
				controlsShown = false;
			} else poke();
		}, 300);
	}

	function onSurfaceDblClick() {
		if (lastPointer === 'mouse' && !isAudio) toggleFullscreen();
	}

	// ——— Keyboard ———
	function onKey(e: KeyboardEvent) {
		if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return;
		const target = e.target as HTMLElement | null;
		const tag = target?.tagName;
		if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target?.isContentEditable) {
			if (e.key !== 'Escape') return;
		}
		const onControl = !!target?.closest('button, [role="radio"], [role="switch"], a');
		const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

		if (key === 'Escape') {
			if (panelOpen) closePanel();
			else if (fullscreen) toggleFullscreen();
			else onback(item);
			e.preventDefault();
			return;
		}
		if (phase === 'prompt' || phase === 'error') return;
		poke();
		switch (key) {
			case ' ':
			case 'Enter':
				if (onControl) return; // native activation of the focused control
				if (key === 'Enter') return;
				togglePlay();
				break;
			case 'k':
				togglePlay();
				break;
			case 'ArrowLeft':
			case 'j':
				if (panelOpen && key === 'ArrowLeft') return;
				seekBy(-10);
				break;
			case 'ArrowRight':
			case 'l':
				if (panelOpen && key === 'ArrowRight') return;
				seekBy(10);
				break;
			case 'ArrowUp':
				if (panelOpen) return;
				setVolume(volume + 0.05);
				break;
			case 'ArrowDown':
				if (panelOpen) return;
				setVolume(volume - 0.05);
				break;
			case 'm':
				toggleMute();
				break;
			case 'f':
				if (!isAudio) toggleFullscreen();
				break;
			case 'c':
				cycleSubtitles();
				break;
			case 'i':
				setStats(!showStats);
				break;
			case 'n':
				if (nextItem) goNext();
				break;
			default:
				if (/^[0-9]$/.test(key) && duration) {
					seek((Number(key) / 10) * duration);
					break;
				}
				return;
		}
		e.preventDefault();
	}

	// ——— Media Session (lock screen, headset keys) ———
	function setMediaSession(d: ItemDetail) {
		if (!('mediaSession' in navigator)) return;
		try {
			const art = firstImage(d, 'poster', 'thumb', 'backdrop');
			navigator.mediaSession.metadata = new MediaMetadata({
				title: d.kind === 'episode' ? `${episodeCode(d) ?? ''} ${d.title}`.trim() : d.title,
				artist: d.ancestors[0]?.title ?? '',
				album: d.kind === 'track' ? (d.ancestors.find((a) => a.kind === 'album')?.title ?? '') : '',
				artwork: art ? [{ src: imageUrl(art, 500) ?? art.url, sizes: '500x500' }] : []
			});
			const h: [MediaSessionAction, MediaSessionActionHandler][] = [
				['play', () => play()],
				['pause', () => video?.pause()],
				['seekbackward', () => seekBy(-10)],
				['seekforward', () => seekBy(10)],
				['seekto', (a) => a.seekTime !== undefined && seek(a.seekTime)],
				['nexttrack', () => goNext()]
			];
			for (const [action, fn] of h) navigator.mediaSession.setActionHandler(action, fn);
		} catch {
			/* partial support */
		}
	}

	// ——— Mount / leave ———
	function onPageHide() {
		teardown(true);
	}

	function onVisibility() {
		if (document.visibilityState === 'hidden') saveNow();
	}

	let statsTimer: ReturnType<typeof setInterval> | undefined;
	$effect(() => {
		if (!showStats) return;
		const tickStats = () => {
			stats = engine?.stats() ?? {};
			updateBuffered();
		};
		tickStats();
		statsTimer = setInterval(tickStats, 1000);
		return () => clearInterval(statsTimer);
	});

	// Keep the controls up while paused, and restart the hide timer when playback resumes.
	$effect(() => {
		if (!paused && phase === 'playing') poke();
	});

	onMount(() => {
		if (video) {
			video.volume = volume;
			video.muted = muted;
		}
		document.addEventListener('fullscreenchange', onFullscreenChange);
		document.addEventListener('webkitfullscreenchange', onFullscreenChange);
		document.addEventListener('visibilitychange', onVisibility);
		window.addEventListener('pagehide', onPageHide);
		loadItem();
		poke();
	});

	onDestroy(() => {
		destroyed = true;
		token++;
		teardown(true);
		clearTimeout(hideTimer);
		clearTimeout(osdTimer);
		clearTimeout(tapTimer);
		if (typeof document === 'undefined') return;
		document.removeEventListener('fullscreenchange', onFullscreenChange);
		document.removeEventListener('webkitfullscreenchange', onFullscreenChange);
		document.removeEventListener('visibilitychange', onVisibility);
		window.removeEventListener('pagehide', onPageHide);
		if ((document as FsDocument).fullscreenElement) document.exitFullscreen().catch(() => {});
		if ('mediaSession' in navigator) navigator.mediaSession.metadata = null;
	});
</script>

<svelte:window onkeydown={onKey} />

<svelte:head>
	<title>{item ? `${heading} — Lecture` : 'Lecture'} — Aether</title>
</svelte:head>

<div
	class="player subs-{subSize}"
	class:audio={isAudio}
	class:idle={!controlsVisible}
	class:panel-open={panelOpen}
	data-theme="nuit"
	bind:this={root}
	onpointermove={(e) => e.pointerType === 'mouse' && poke()}
	role="region"
	aria-label="Lecteur{item ? ` : ${item.title}` : ''}"
>
	<!-- svelte-ignore a11y_media_has_caption (text tracks are added per session) -->
	<video
		class="media"
		class:lifted={controlsVisible}
		bind:this={video}
		playsinline
		preload="auto"
		onloadedmetadata={onMeta}
		ontimeupdate={onTime}
		onprogress={updateBuffered}
		onplay={() => (paused = false)}
		onpause={() => {
			paused = true;
			saveNow();
		}}
		onplaying={onPlaying}
		onwaiting={() => (waiting = true)}
		oncanplay={() => (waiting = false)}
		onseeked={() => (restarting = false)}
		onloadeddata={() => {
			if (video?.paused && !(info?.startMs ?? 0)) restarting = false;
			if (video?.paused) firstFrame = true;
		}}
		onended={onEnded}
		onvolumechange={onVolumeChange}
	>
		{#each vttTracks as t (`${info?.sessionId}-${t.index}`)}
			<track kind="subtitles" src={t.url} srclang={t.language ?? 'und'} label={subtitleTrackLabel(t).main} data-index={t.index} onload={onTrackLoad} />
		{/each}
	</video>

	<canvas class="snapshot" class:on={restarting} bind:this={snapshot} aria-hidden="true"></canvas>

	<!-- Backdrop while loading, behind audio, and around prompts / errors. -->
	{#if isAudio || !firstFrame || phase === 'error' || phase === 'prompt'}
		<div class="backdrop" class:blurred={isAudio} aria-hidden="true">
			{#if backdrop}<Artwork image={backdrop} fill sizes="100vw" maxWidth={1920} eager />{/if}
			<div class="veil"></div>
		</div>
	{/if}

	{#if isAudio && item}
		<div class="cover-stage">
			<div class="cover">
				<Artwork image={cover} ratio="1 / 1" sizes="(max-width: 767px) 60vw, 340px" eager placeholder={{ title: item.title, icon: 'music' }} alt="Pochette de {albumTitle ?? item.title}" />
			</div>
			<p class="track-title">{item.title}</p>
			{#if eyebrow}<p class="track-sub">{eyebrow}</p>{/if}
		</div>
	{/if}

	<!-- Click / tap surface (under the controls). -->
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions (keyboard: window shortcuts) -->
	<div class="surface" onpointerdown={onSurfaceDown} onclick={onSurfaceClick} ondblclick={onSurfaceDblClick}></div>

	{#if spinning && phase !== 'error' && phase !== 'prompt'}
		<div class="spinner" role="status" aria-label={recovering ? 'Reconnexion…' : 'Chargement…'}><span></span></div>
	{/if}

	{#if needsGesture && phase === 'playing'}
		<button type="button" class="big-play" aria-label="Lecture" onclick={() => play()}><Icon name="play" size={34} filled /></button>
	{/if}

	{#if osd}
		{#key osd.key}
			<div class="osd" class:left={osd.side === 'left'} class:right={osd.side === 'right'} aria-hidden="true">
				{#if osd.icon}<Icon name={osd.icon} size={osd.side ? 26 : 30} filled={osd.icon === 'play' || osd.icon === 'pause'} />{/if}
				{#if osd.text}<span>{osd.text}</span>{/if}
			</div>
		{/key}
	{/if}

	<!-- Top: back, title. -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="top chrome" onmouseenter={() => (hoveringControls = true)} onmouseleave={() => (hoveringControls = false)}>
		<button type="button" class="ctl back" aria-label="Retour" onclick={() => onback(item)}><Icon name="arrow-left" size={24} /></button>
		{#if item && !isAudio}
			<div class="titles">
				{#if eyebrow}<p class="eyebrow">{eyebrow}</p>{/if}
				<h1>{heading}</h1>
			</div>
		{/if}
	</div>

	{#if showStats && info && phase === 'playing'}
		<div class="stats-wrap">
			<PlaybackStats
				{info}
				engine={engineKind}
				{stats}
				{bufferAhead}
				videoSize={video ? { width: video.videoWidth, height: video.videoHeight } : undefined}
				{fileBitrate}
				audioLabel={audioTrack ? Object.values(audioTrackLabel(audioTrack)).filter(Boolean).join(' · ') : undefined}
			/>
		</div>
	{/if}

	{#if showNext && nextItem}
		<div class="next" class:prominent={ended}>
			<Button variant="primary" icon="skip-next" onclick={goNext} aria-label="Épisode suivant : {nextLabel}">Épisode suivant</Button>
			{#if ended}<p class="next-label">{nextLabel}</p>{/if}
		</div>
	{/if}

	<!-- Bottom: seek bar and transport. -->
	{#if phase === 'playing' || phase === 'starting'}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="bottom chrome" onmouseenter={() => (hoveringControls = true)} onmouseleave={() => (hoveringControls = false)}>
			<SeekBar
				{current}
				{duration}
				{buffered}
				onseek={(t) => {
					seek(t);
					poke();
				}}
				onscrub={(a) => (scrubbing = a)}
			/>
			<div class="row">
				<div class="group">
					<button type="button" class="ctl primary" aria-label={paused ? 'Lecture' : 'Pause'} onclick={togglePlay}>
						<Icon name={paused ? 'play' : 'pause'} size={26} filled />
					</button>
					<button type="button" class="ctl" aria-label="Reculer de 10 secondes" onclick={() => seekBy(-10)}><Icon name="back-10" size={24} /></button>
					<button type="button" class="ctl" aria-label="Avancer de 10 secondes" onclick={() => seekBy(10)}><Icon name="forward-10" size={24} /></button>
					<div class="volume">
						<button type="button" class="ctl" aria-label={muted ? 'Rétablir le son' : 'Couper le son'} aria-pressed={muted} onclick={toggleMute}>
							<Icon name={volumeIcon} size={24} />
						</button>
						<input
							type="range"
							min="0"
							max="1"
							step="0.05"
							value={muted ? 0 : volume}
							aria-label="Volume"
							aria-valuetext="{Math.round((muted ? 0 : volume) * 100)} %"
							oninput={(e) => setVolume(Number(e.currentTarget.value))}
							style:--fill="{(muted ? 0 : volume) * 100}%"
						/>
					</div>
					<span class="time tabular"><span>{formatClock(current)}</span><span class="sep">/</span><span class="total">{formatClock(duration)}</span></span>
				</div>
				<div class="group">
					{#if nextItem && !showNext}
						<button type="button" class="ctl" aria-label="{isAudio ? 'Piste suivante' : 'Épisode suivant'} : {nextLabel || nextItem.title}" onclick={goNext}><Icon name="skip-next" size={24} /></button>
					{/if}
					{#if !isAudio}
						<button
							type="button"
							class="ctl"
							class:on={activeSub >= 0}
							aria-label="Sous-titres{activeSub >= 0 ? ' (activés)' : ''}"
							onclick={() => openPanel('subtitles')}
							disabled={!info}
						>
							<Icon name="captions" size={24} />
						</button>
					{/if}
					<button
						type="button"
						class="ctl"
						bind:this={panelButton}
						aria-label="Réglages : audio, sous-titres, qualité"
						aria-expanded={panelOpen}
						onclick={() => (panelOpen ? closePanel() : openPanel())}
						disabled={!info}
					>
						<Icon name="settings" size={24} />
					</button>
					{#if !isAudio}
						<button type="button" class="ctl" aria-label={fullscreen ? 'Quitter le plein écran' : 'Plein écran'} onclick={toggleFullscreen}>
							<Icon name={fullscreen ? 'minimize' : 'maximize'} size={24} />
						</button>
					{/if}
				</div>
			</div>
		</div>
	{/if}

	{#if panelOpen && info}
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions (Esc closes) -->
		<div class="panel-scrim" onclick={closePanel}></div>
		<div class="panel-wrap">
			<PlayerPanel
				audioTracks={info.audioTracks}
				subtitleTracks={info.subtitleTracks}
				{selectedAudio}
				selectedSubtitle={activeSub}
				{quality}
				currentHeight={info.video?.height ?? video?.videoHeight}
				subtitleSize={subSize}
				stats={showStats}
				{isAudio}
				focus={panelFocus}
				onaudio={selectAudio}
				onsubtitle={selectSubtitle}
				onquality={selectQuality}
				onsubtitlesize={setSubSize}
				onstats={setStats}
				onclose={closePanel}
			/>
		</div>
	{/if}

	{#if phase === 'prompt'}
		<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="resume-title">
			<p class="eyebrow">{eyebrow ?? ''}</p>
			<h2 id="resume-title">Reprendre à {formatClock(resumeMs / 1000)} ?</h2>
			<p class="muted">{heading}</p>
			{#if duration}
				<div class="resume-line" aria-hidden="true"><span style:width="{Math.min(100, (resumeMs / 1000 / duration) * 100)}%"></span></div>
			{/if}
			<div class="actions">
				<!-- svelte-ignore a11y_autofocus -->
				<Button variant="primary" size="lg" icon="play" iconFilled autofocus onclick={() => chooseResume(true)}>Reprendre</Button>
				<Button size="lg" icon="refresh" onclick={() => chooseResume(false)}>Depuis le début</Button>
			</div>
		</div>
	{/if}

	{#if phase === 'error'}
		<div class="dialog" role="alertdialog" aria-modal="true" aria-labelledby="err-title" aria-describedby="err-text">
			<span class="err-icon"><Icon name="alert" size={26} /></span>
			<h2 id="err-title">Lecture impossible</h2>
			<p id="err-text" class="muted">{errorText}</p>
			<div class="actions">
				<!-- svelte-ignore a11y_autofocus -->
				<Button variant="primary" size="lg" icon="refresh" autofocus onclick={retry}>Réessayer</Button>
				<Button size="lg" icon="chevron-left" onclick={() => onback(item)}>Retour</Button>
			</div>
		</div>
	{/if}
</div>

<style>
	.player {
		--player-bg: #02080d;
		position: fixed;
		inset: 0;
		z-index: 50;
		overflow: hidden;
		background: var(--player-bg);
		color: var(--on-image);
		user-select: none;
		-webkit-user-select: none;
		-webkit-tap-highlight-color: transparent;
	}

	.player.idle {
		cursor: none;
	}

	.media,
	.snapshot {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		max-width: none;
		object-fit: contain;
		background: transparent;
	}

	.audio .media {
		opacity: 0;
		pointer-events: none;
	}

	.snapshot {
		opacity: 0;
		pointer-events: none;
	}

	.snapshot.on {
		opacity: 1;
	}

	/* ——— Subtitles ——— */
	.media::cue {
		font-family: var(--font-ui);
		font-weight: 700;
		line-height: 1.3;
		color: #f6ffff;
		background: rgba(3, 13, 22, 0.66);
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.35);
		/* Relative to the browser's cue box, which already scales with the video height. */
		font-size: 78%;
	}

	.subs-s .media::cue {
		font-size: 62%;
	}

	.subs-l .media::cue {
		font-size: 100%;
	}

	/* Chromium/WebKit: lift the cues above the control bar while it is shown. */
	.media::-webkit-media-text-track-container {
		transition: transform var(--dur-base) var(--ease);
	}

	.media.lifted::-webkit-media-text-track-container {
		transform: translateY(-84px);
	}

	/* ——— Backdrop & audio ——— */
	.backdrop {
		position: absolute;
		inset: 0;
		background: var(--player-bg);
	}

	.backdrop :global(img) {
		opacity: 0.55;
	}

	.backdrop.blurred :global(img) {
		filter: blur(42px) saturate(1.1);
		transform: scale(1.15);
		opacity: 0.5;
	}

	.veil {
		position: absolute;
		inset: 0;
		background:
			radial-gradient(80% 80% at 50% 45%, rgba(2, 8, 13, 0.25), rgba(2, 8, 13, 0.85)),
			linear-gradient(to top, rgba(2, 8, 13, 0.9), transparent 45%);
	}

	.cover-stage {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--sp-2);
		padding: var(--sp-8) var(--sp-5) 150px;
		pointer-events: none;
		text-align: center;
	}

	.cover {
		width: min(340px, 62vw, 46vh);
		margin-bottom: var(--sp-4);
		border-radius: var(--r-lg);
		overflow: hidden;
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
	}

	.track-title {
		font-size: var(--fs-h2);
		font-weight: var(--fw-heavy);
	}

	.track-sub {
		color: var(--on-image-muted);
		font-weight: var(--fw-medium);
	}

	.surface {
		position: absolute;
		inset: 0;
	}

	/* ——— Spinner, OSD, big play ——— */
	.spinner {
		position: absolute;
		left: 50%;
		top: 50%;
		width: 52px;
		height: 52px;
		margin: -26px 0 0 -26px;
		pointer-events: none;
	}

	.spinner span {
		position: absolute;
		inset: 0;
		border-radius: 50%;
		border: 3px solid rgba(246, 255, 255, 0.14);
		border-top-color: var(--accent);
		animation: spin 0.9s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.big-play {
		position: absolute;
		left: 50%;
		top: 50%;
		display: grid;
		place-items: center;
		width: 84px;
		height: 84px;
		margin: -42px 0 0 -42px;
		padding-left: 4px;
		border-radius: 50%;
		background: var(--accent);
		color: var(--accent-contrast);
		box-shadow: var(--shadow-2);
	}

	.osd {
		position: absolute;
		left: 50%;
		top: 50%;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
		min-width: 84px;
		padding: var(--sp-4);
		border-radius: var(--r-lg);
		background: rgba(3, 13, 22, 0.6);
		transform: translate(-50%, -50%);
		font-weight: var(--fw-bold);
		font-size: var(--fs-small);
		pointer-events: none;
		animation: osd 700ms var(--ease) forwards;
	}

	.osd.left {
		left: 18%;
	}

	.osd.right {
		left: 82%;
	}

	@keyframes osd {
		0% {
			opacity: 0;
			transform: translate(-50%, -50%) scale(0.92);
		}
		15%,
		65% {
			opacity: 1;
			transform: translate(-50%, -50%) scale(1);
		}
		100% {
			opacity: 0;
			transform: translate(-50%, -50%) scale(1);
		}
	}

	/* ——— Chrome (top & bottom bars) ——— */
	.chrome {
		position: absolute;
		left: 0;
		right: 0;
		padding-left: max(var(--sp-5), env(safe-area-inset-left, 0px));
		padding-right: max(var(--sp-5), env(safe-area-inset-right, 0px));
		transition:
			opacity var(--dur-base) var(--ease),
			transform var(--dur-base) var(--ease);
	}

	.idle .chrome,
	.idle .chrome > :global(*),
	.idle .next,
	.idle .stats-wrap {
		opacity: 0;
		pointer-events: none;
	}

	.idle .top {
		transform: translateY(-8px);
	}

	.idle .bottom {
		transform: translateY(8px);
	}

	/* The gradients are decoration: clicks through them reach the video surface. */
	.chrome {
		pointer-events: none;
	}

	.chrome > :global(*) {
		pointer-events: auto;
	}

	.top {
		top: 0;
		display: flex;
		align-items: flex-start;
		gap: var(--sp-3);
		padding-top: max(var(--sp-4), env(safe-area-inset-top, 0px));
		padding-bottom: var(--sp-8);
		background: linear-gradient(to bottom, rgba(2, 8, 13, 0.78), rgba(2, 8, 13, 0.36) 55%, transparent);
	}

	.titles {
		min-width: 0;
		padding-top: 4px;
	}

	.eyebrow {
		font-size: var(--fs-micro);
		font-weight: var(--fw-bold);
		color: var(--on-image-muted);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	h1 {
		font-size: clamp(1rem, 0.85rem + 0.6vw, 1.375rem);
		line-height: 1.3;
		font-weight: var(--fw-heavy);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.bottom {
		bottom: 0;
		padding-top: var(--sp-9);
		padding-bottom: max(var(--sp-3), env(safe-area-inset-bottom, 0px));
		background: linear-gradient(to top, rgba(2, 8, 13, 0.86), rgba(2, 8, 13, 0.45) 55%, transparent);
	}

	.row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sp-3);
		margin-top: 2px;
	}

	.group {
		display: flex;
		align-items: center;
		gap: 2px;
		min-width: 0;
	}

	.ctl {
		display: grid;
		place-items: center;
		flex: none;
		width: 44px;
		height: 44px;
		border-radius: var(--r-md);
		color: var(--on-image);
		transition:
			background-color var(--dur-fast) var(--ease),
			color var(--dur-fast) var(--ease);
	}

	.ctl:hover:not(:disabled) {
		background: rgba(246, 255, 255, 0.1);
	}

	.ctl:disabled {
		opacity: 0.4;
	}

	.ctl.on {
		color: var(--accent);
	}

	.volume {
		display: flex;
		align-items: center;
	}

	.volume input {
		width: 0;
		height: 24px;
		margin: 0;
		opacity: 0;
		appearance: none;
		-webkit-appearance: none;
		background: transparent;
		cursor: pointer;
		transition:
			width var(--dur-base) var(--ease),
			opacity var(--dur-base) var(--ease),
			margin var(--dur-base) var(--ease);
	}

	.volume:hover input,
	.volume:focus-within input {
		width: 88px;
		margin: 0 var(--sp-2) 0 2px;
		opacity: 1;
	}

	.volume input::-webkit-slider-runnable-track {
		height: 4px;
		border-radius: 2px;
		background: linear-gradient(to right, var(--on-image) var(--fill), rgba(246, 255, 255, 0.24) var(--fill));
	}

	.volume input::-moz-range-track {
		height: 4px;
		border-radius: 2px;
		background: linear-gradient(to right, var(--on-image) var(--fill), rgba(246, 255, 255, 0.24) var(--fill));
	}

	.volume input::-webkit-slider-thumb {
		-webkit-appearance: none;
		width: 12px;
		height: 12px;
		margin-top: -4px;
		border-radius: 50%;
		background: var(--on-image);
	}

	.volume input::-moz-range-thumb {
		width: 12px;
		height: 12px;
		border: 0;
		border-radius: 50%;
		background: var(--on-image);
	}

	.time {
		margin-left: var(--sp-2);
		font-size: var(--fs-small);
		font-weight: var(--fw-bold);
		white-space: nowrap;
	}

	.time .sep,
	.time .total {
		color: var(--on-image-muted);
	}

	.time .sep {
		margin: 0 0.3em;
	}

	/* ——— Next episode, stats ——— */
	.next {
		position: absolute;
		right: max(var(--sp-5), env(safe-area-inset-right, 0px));
		bottom: calc(118px + env(safe-area-inset-bottom, 0px));
		display: grid;
		justify-items: end;
		gap: var(--sp-2);
		z-index: 1;
		transition: opacity var(--dur-base) var(--ease);
	}

	.next.prominent {
		opacity: 1 !important;
		pointer-events: auto !important;
	}

	.next-label {
		max-width: 320px;
		font-weight: var(--fw-bold);
		text-align: right;
		text-shadow: 0 1px 8px rgba(0, 0, 0, 0.5);
	}

	.stats-wrap {
		position: absolute;
		top: calc(84px + env(safe-area-inset-top, 0px));
		left: max(var(--sp-5), env(safe-area-inset-left, 0px));
		transition: opacity var(--dur-base) var(--ease);
	}

	/* ——— Side panel ——— */
	.panel-scrim {
		position: absolute;
		inset: 0;
		background: rgba(2, 8, 13, 0.3);
	}

	.panel-wrap {
		position: absolute;
		top: 0;
		right: 0;
		bottom: 0;
		display: flex;
		justify-content: flex-end;
		width: min(360px, 100%);
		animation: slide var(--dur-base) var(--ease);
	}

	@keyframes slide {
		from {
			transform: translateX(24px);
			opacity: 0;
		}
	}

	/* ——— Dialogs (resume, error) ——— */
	.dialog {
		position: absolute;
		left: 50%;
		top: 50%;
		display: grid;
		justify-items: center;
		gap: var(--sp-3);
		width: min(480px, calc(100% - 32px));
		padding: var(--sp-6);
		border-radius: var(--r-xl);
		background: var(--tint), rgba(7, 23, 37, 0.9);
		box-shadow: var(--shadow-3), inset 0 0 0 1px var(--line);
		transform: translate(-50%, -50%);
		text-align: center;
		color: var(--text);
	}

	.dialog h2 {
		font-size: var(--fs-hero);
		line-height: var(--lh-hero);
		font-weight: var(--fw-heavy);
		letter-spacing: -0.01em;
	}

	.muted {
		color: var(--text-muted);
	}

	.resume-line {
		width: 100%;
		max-width: 280px;
		height: 4px;
		margin: var(--sp-1) 0;
		border-radius: var(--r-full);
		background: rgba(246, 255, 255, 0.14);
		overflow: hidden;
	}

	.resume-line span {
		display: block;
		height: 100%;
		background: var(--accent);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--sp-3);
		margin-top: var(--sp-3);
	}

	.err-icon {
		display: grid;
		place-items: center;
		width: 52px;
		height: 52px;
		border-radius: 50%;
		background: color-mix(in srgb, var(--danger) 16%, transparent);
		color: var(--danger);
	}

	/* ——— Small screens ——— */
	@media (max-width: 767px), (max-height: 500px) {
		.chrome {
			padding-left: max(var(--sp-3), env(safe-area-inset-left, 0px));
			padding-right: max(var(--sp-3), env(safe-area-inset-right, 0px));
		}

		.bottom {
			padding-top: var(--sp-7);
		}

		.top {
			padding-bottom: var(--sp-6);
		}

		.media.lifted::-webkit-media-text-track-container {
			transform: translateY(-64px);
		}

		.next {
			bottom: calc(104px + env(safe-area-inset-bottom, 0px));
			right: max(var(--sp-3), env(safe-area-inset-right, 0px));
		}

		.dialog {
			padding: var(--sp-5);
		}

		.dialog h2 {
			font-size: 1.5rem;
		}
	}

	@media (pointer: coarse) {
		.volume input {
			display: none;
		}
	}

	@media (max-width: 480px) {
		.time .sep,
		.time .total {
			display: none;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.chrome,
		.next,
		.stats-wrap,
		.volume input,
		.media::-webkit-media-text-track-container {
			transition: opacity var(--dur-fast) linear;
		}

		.idle .top,
		.idle .bottom {
			transform: none;
		}

		.spinner span {
			animation: pulse 1.6s ease-in-out infinite;
			border-color: var(--accent);
		}

		@keyframes pulse {
			50% {
				opacity: 0.35;
			}
		}

		.osd,
		.panel-wrap {
			animation: none;
		}
	}
</style>
