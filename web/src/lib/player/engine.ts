// Media engine: attaches a PlaybackInfo to a <video> element.
// - "direct": progressive file (HTTP Range) on video.src.
// - HLS with profile.hls === "native" (Safari): master.m3u8 on video.src.
// - HLS with "mse": hls.js (light build: no alt-audio/subtitle renditions, no EME — audio and
//   subtitle changes go through a new session, text subtitles through <track>). Lazy-loaded,
//   so direct play never downloads it.
//
// Error policy (hls.js): media errors → recoverMediaError(), then swapAudioCodec() +
// recoverMediaError(), then fatal "media". Network errors → hls.js' own retries (generous
// timeouts: segments are produced on demand), then startLoad() with exponential backoff
// (1, 2, 4, 8 s), then fatal "network". Any other fatal error → fatal "other".

import type { PlaybackInfo } from '$lib/api/types';
import type HlsType from 'hls.js';
import type { ErrorData } from 'hls.js';
import { PlayerError } from './labels';

export interface EngineStats {
	/** hls.js bandwidth estimate (bits/s). */
	bandwidth?: number;
	/** Bitrate advertised by the current HLS variant (bits/s). */
	levelBitrate?: number;
	droppedFrames?: number;
	totalFrames?: number;
}

export interface Engine {
	readonly kind: 'direct' | 'native-hls' | 'hls.js';
	stats(): EngineStats;
	destroy(): void;
}

export interface EngineOptions {
	hls: 'native' | 'mse';
	/** Seconds on the media timeline to start at. */
	startSeconds: number;
	onFatal(err: PlayerError): void;
	/** Transient trouble being handled (spinner text). */
	onRecovering?(what: 'network' | 'media'): void;
}

const RETRY_DELAYS = [1000, 2000, 4000, 8000];

function frames(video: HTMLVideoElement): Pick<EngineStats, 'droppedFrames' | 'totalFrames'> {
	const q = video.getVideoPlaybackQuality?.();
	return q ? { droppedFrames: q.droppedVideoFrames, totalFrames: q.totalVideoFrames } : {};
}

function elementError(video: HTMLVideoElement): PlayerError {
	const code = video.error?.code;
	if (code === MediaError.MEDIA_ERR_NETWORK) return new PlayerError('network', 'La connexion au serveur a été interrompue.');
	if (code === MediaError.MEDIA_ERR_DECODE) return new PlayerError('media', 'Le navigateur n’a pas pu décoder ce média.');
	if (code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED) return new PlayerError('unsupported', 'Ce format n’est pas pris en charge par le navigateur.');
	return new PlayerError('other', 'La lecture a échoué.');
}

/** Progressive source or native HLS: the element does everything. */
function attachElement(video: HTMLVideoElement, url: string, kind: 'direct' | 'native-hls', opts: EngineOptions): Engine {
	let disposed = false;
	const onError = () => {
		if (!disposed && video.error) opts.onFatal(elementError(video));
	};
	const onMeta = () => {
		if (opts.startSeconds > 0 && Math.abs(video.currentTime - opts.startSeconds) > 0.5) video.currentTime = opts.startSeconds;
	};
	video.addEventListener('error', onError);
	video.addEventListener('loadedmetadata', onMeta, { once: true });
	video.src = url;
	video.load();
	return {
		kind,
		stats: () => frames(video),
		destroy() {
			disposed = true;
			video.removeEventListener('error', onError);
			video.removeEventListener('loadedmetadata', onMeta);
			video.removeAttribute('src');
			video.load();
		}
	};
}

let hlsModule: Promise<typeof HlsType> | null = null;

/** hls.js light build, fetched on first use only. */
export function loadHls(): Promise<typeof HlsType> {
	hlsModule ??= import('hls.js/light').then((m) => m.default as typeof HlsType);
	return hlsModule;
}

async function attachHlsJs(video: HTMLVideoElement, url: string, opts: EngineOptions): Promise<Engine> {
	const Hls = await loadHls();
	if (!Hls.isSupported()) throw new PlayerError('unsupported', 'Ce navigateur ne prend pas en charge la lecture adaptative (Media Source Extensions).');

	const loadPolicy = (firstByte: number, total: number, retries: number) => ({
		default: {
			maxTimeToFirstByteMs: firstByte,
			maxLoadTimeMs: total,
			timeoutRetry: { maxNumRetry: retries, retryDelayMs: 0, maxRetryDelayMs: 0 },
			errorRetry: { maxNumRetry: retries, retryDelayMs: 1000, maxRetryDelayMs: 8000, backoff: 'exponential' as const }
		}
	});

	const hls = new Hls({
		startPosition: opts.startSeconds > 0 ? opts.startSeconds : -1,
		// Segments are produced on demand by FFmpeg: be patient with the first byte.
		manifestLoadPolicy: loadPolicy(20_000, 30_000, 2),
		playlistLoadPolicy: loadPolicy(20_000, 30_000, 2),
		fragLoadPolicy: loadPolicy(30_000, 90_000, 3),
		maxBufferLength: 30,
		maxMaxBufferLength: 90,
		backBufferLength: 60,
		enableWorker: true,
		lowLatencyMode: false,
		capLevelToPlayerSize: false
	});

	let disposed = false;
	let networkAttempts = 0;
	let mediaRecoveries = 0;
	let lastMediaRecovery = 0;
	let retryTimer: ReturnType<typeof setTimeout> | undefined;

	const fail = (err: PlayerError) => {
		if (disposed) return;
		opts.onFatal(err);
	};

	hls.on(Hls.Events.FRAG_LOADED, () => {
		networkAttempts = 0;
	});

	hls.on(Hls.Events.ERROR, (_e, data: ErrorData) => {
		if (disposed || !data.fatal) return;
		if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
			const status = data.response?.code;
			// Session gone (stopped server-side): retrying the same URLs is pointless.
			if (status === 404 || status === 410) return fail(new PlayerError('network', 'La session de lecture a expiré.'));
			if (networkAttempts < RETRY_DELAYS.length) {
				opts.onRecovering?.('network');
				clearTimeout(retryTimer);
				retryTimer = setTimeout(() => !disposed && hls.startLoad(), RETRY_DELAYS[networkAttempts++]);
				return;
			}
			return fail(new PlayerError('network', 'La connexion au serveur a été perdue.'));
		}
		if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
			const now = performance.now();
			if (now - lastMediaRecovery > 10_000) mediaRecoveries = 0;
			lastMediaRecovery = now;
			opts.onRecovering?.('media');
			if (mediaRecoveries === 0) {
				mediaRecoveries++;
				hls.recoverMediaError();
				return;
			}
			if (mediaRecoveries === 1) {
				mediaRecoveries++;
				hls.swapAudioCodec();
				hls.recoverMediaError();
				return;
			}
			return fail(new PlayerError('media', 'Le navigateur n’a pas pu décoder ce flux.'));
		}
		fail(new PlayerError('other', `La lecture a échoué (${data.details}).`));
	});

	hls.attachMedia(video);
	hls.loadSource(url);

	return {
		kind: 'hls.js',
		stats: () => {
			const level = hls.levels?.[hls.currentLevel];
			return {
				bandwidth: Number.isFinite(hls.bandwidthEstimate) ? hls.bandwidthEstimate : undefined,
				levelBitrate: level?.bitrate || undefined,
				...frames(video)
			};
		},
		destroy() {
			disposed = true;
			clearTimeout(retryTimer);
			hls.destroy();
			video.removeAttribute('src');
			video.load();
		}
	};
}

export async function attachSource(video: HTMLVideoElement, info: PlaybackInfo, opts: EngineOptions): Promise<Engine> {
	if (info.method === 'direct') return attachElement(video, info.url, 'direct', opts);
	if (opts.hls === 'native') return attachElement(video, info.url, 'native-hls', opts);
	return attachHlsJs(video, info.url, opts);
}
