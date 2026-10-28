// Client playback profile (docs/API.md « Lecture (M5) »).
//
// `buildProfile` is pure: every browser query goes through injected probes, so it can be
// tested without a DOM (see capabilities.test.ts). `browserProbes()` wires the real ones.
//
// Rules, deliberately conservative — claiming too much means a black screen, claiming too
// little only costs a remux/transcode:
// - A container is claimed when the media element says it can play it (`canPlayType`),
//   never "mkv" (Chrome plays some Matroska files as WebM but not reliably).
// - A video codec is claimed when the element can play it progressively AND it can be fed
//   through the HLS path the server would use for a remux: MSE `video/mp4` (hls.js) or the
//   native player's `video/mp4` (Safari). VP8 has no fMP4 mapping, so it is never claimed.
// - An audio codec is claimed when playable in fMP4 (same rule), or — for codecs mostly met
//   in audio files (mp3, vorbis, pcm) — when the element plays their usual container.
// - HDR: the display reports `(dynamic-range: high)` and a 10-bit codec (HEVC Main 10,
//   AV1 Main 10 or VP9 profile 2) is decodable.
// - maxHeight: the smallest ladder rung covering the screen in device pixels (16:9 frame),
//   or the user's quality choice when lower.
// - hls: "native" for Safari (and any browser without MSE that plays HLS), else "mse".

import type { ClientProfile } from '$lib/api/types';

export interface CapabilityProbes {
	/** HTMLMediaElement.canPlayType on a <video> element ("" | "maybe" | "probably"). */
	canPlayType(mime: string): string;
	/** MediaSource.isTypeSupported (or ManagedMediaSource); undefined when MSE is absent. */
	isTypeSupported?: (mime: string) => boolean;
	/** matchMedia('(dynamic-range: high)').matches */
	hdrDisplay: boolean;
	/** CSS pixels and devicePixelRatio. */
	screen: { width: number; height: number; dpr: number };
	/** Safari (desktop or iOS), where native HLS is preferred over MSE. */
	safari: boolean;
}

export type QualityChoice = 'auto' | '1080p' | '720p' | '480p';

export const QUALITY_PRESETS: Record<QualityChoice, { label: string; maxHeight?: number; maxBitrate: number }> = {
	auto: { label: 'Auto', maxBitrate: 0 },
	'1080p': { label: '1080p', maxHeight: 1080, maxBitrate: 10_000_000 },
	'720p': { label: '720p', maxHeight: 720, maxBitrate: 4_000_000 },
	'480p': { label: '480p', maxHeight: 480, maxBitrate: 1_500_000 }
};

export const HEIGHT_LADDER = [480, 720, 1080, 1440, 2160] as const;

/** MIME strings with precise codec parameters. */
export const MIME = {
	h264: ['video/mp4; codecs="avc1.640028"', 'video/mp4; codecs="avc1.4d401f"'],
	hevc: ['video/mp4; codecs="hvc1.1.6.L120.90"', 'video/mp4; codecs="hev1.1.6.L120.90"'],
	hevc10: ['video/mp4; codecs="hvc1.2.4.L153.B0"', 'video/mp4; codecs="hev1.2.4.L153.B0"'],
	av1: ['video/mp4; codecs="av01.0.08M.08"', 'video/webm; codecs="av01.0.08M.08"'],
	av1_10: ['video/mp4; codecs="av01.0.09M.10"', 'video/webm; codecs="av01.0.09M.10"'],
	vp9: ['video/mp4; codecs="vp09.00.10.08"', 'video/webm; codecs="vp09.00.10.08"', 'video/webm; codecs="vp9"'],
	vp9_10: ['video/mp4; codecs="vp09.02.10.10"', 'video/webm; codecs="vp09.02.10.10"'],
	aac: ['audio/mp4; codecs="mp4a.40.2"'],
	mp3: ['audio/mp4; codecs="mp4a.6B"', 'audio/mp4; codecs="mp4a.69"', 'audio/mpeg'],
	opus: ['audio/mp4; codecs="opus"', 'audio/webm; codecs="opus"'],
	flac: ['audio/mp4; codecs="flac"', 'audio/flac'],
	ac3: ['audio/mp4; codecs="ac-3"'],
	eac3: ['audio/mp4; codecs="ec-3"'],
	alac: ['audio/mp4; codecs="alac"'],
	vorbis: ['audio/webm; codecs="vorbis"', 'audio/ogg; codecs="vorbis"'],
	pcm: ['audio/wav; codecs="1"', 'audio/wav']
} as const;

const CONTAINERS: [name: string, mimes: string[]][] = [
	['mp4', ['video/mp4']],
	['webm', ['video/webm']],
	['mov', ['video/quicktime']],
	['ogg', ['audio/ogg', 'video/ogg']],
	['mp3', ['audio/mpeg']],
	['flac', ['audio/flac', 'audio/x-flac']],
	['m4a', ['audio/mp4', 'audio/x-m4a']],
	['wav', ['audio/wav', 'audio/wave']]
];

const isMp4 = (m: string) => m.startsWith('video/mp4') || m.startsWith('audio/mp4');

/** Smallest rung ≥ `px` (2160 past the end). */
export function snapHeight(px: number): number {
	for (const h of HEIGHT_LADDER) if (h >= px - 8) return h;
	return HEIGHT_LADDER[HEIGHT_LADDER.length - 1];
}

/** Height (device pixels) of a 16:9 frame fitted to the screen, whatever its orientation. */
export function screenHeight(screen: CapabilityProbes['screen']): number {
	const dpr = screen.dpr > 0 ? screen.dpr : 1;
	const long = Math.max(screen.width, screen.height) * dpr;
	const short = Math.min(screen.width, screen.height) * dpr;
	return Math.min(short, (long * 9) / 16);
}

export function buildProfile(p: CapabilityProbes, quality: QualityChoice = 'auto'): ClientProfile {
	const element = (m: string) => p.canPlayType(m) !== '';
	const mse = (m: string) => !!p.isTypeSupported?.(m);
	const hlsNativeAvailable = element('application/vnd.apple.mpegurl') || element('application/x-mpegURL');
	const hls: ClientProfile['hls'] = hlsNativeAvailable && (p.safari || !p.isTypeSupported) ? 'native' : 'mse';

	/** Usable over HLS fMP4 (what a remux or transcode produces). */
	const overHls = (mimes: readonly string[]) => mimes.filter(isMp4).some((m) => (hls === 'mse' ? mse(m) : element(m)));
	const progressive = (mimes: readonly string[]) => mimes.some(element);
	const both = (mimes: readonly string[]) => progressive(mimes) && overHls(mimes);

	const videoCodecs: string[] = [];
	if (both(MIME.h264)) videoCodecs.push('h264');
	if (both(MIME.hevc) || both(MIME.hevc10)) videoCodecs.push('hevc');
	if (both(MIME.av1)) videoCodecs.push('av1');
	if (both(MIME.vp9)) videoCodecs.push('vp9');
	// vp8: no fMP4 mapping for a remux — not claimed (see header).

	const audioCodecs: string[] = [];
	for (const c of ['aac', 'opus', 'flac', 'ac3', 'eac3', 'alac'] as const) if (both(MIME[c])) audioCodecs.push(c);
	// Codecs mostly met in audio files (direct play of .mp3 / .ogg / .wav). Chromium's MSE
	// rejects MP3-in-fMP4, so a video remux carrying them can fail: the decode-failure ladder
	// (degradeProfile) then falls back to AAC.
	if (both(MIME.mp3) || progressive(MIME.mp3)) audioCodecs.push('mp3');
	if (progressive(MIME.vorbis)) audioCodecs.push('vorbis');
	if (progressive(MIME.pcm)) audioCodecs.push('pcm');

	const containers = CONTAINERS.filter(([, mimes]) => mimes.some(element)).map(([name]) => name);

	const tenBit = both(MIME.hevc10) || both(MIME.av1_10) || both(MIME.vp9_10);
	const hdr = p.hdrDisplay && tenBit;

	const preset = QUALITY_PRESETS[quality] ?? QUALITY_PRESETS.auto;
	const fromScreen = snapHeight(screenHeight(p.screen));
	const maxHeight = preset.maxHeight ? Math.min(preset.maxHeight, fromScreen) : fromScreen;

	return { containers, videoCodecs, audioCodecs, hdr, maxHeight, maxBitrate: preset.maxBitrate, hls };
}

/** Real probes. Call in the browser only. */
export function browserProbes(): CapabilityProbes {
	const video = document.createElement('video');
	const w = window as unknown as { MediaSource?: typeof MediaSource; ManagedMediaSource?: typeof MediaSource };
	const MS = w.MediaSource ?? w.ManagedMediaSource;
	const ua = navigator.userAgent;
	const safari = /^((?!chrome|chromium|android|crios|fxios|edg).)*safari/i.test(ua);
	return {
		canPlayType: (m) => {
			try {
				return video.canPlayType(m);
			} catch {
				return '';
			}
		},
		isTypeSupported: MS?.isTypeSupported
			? (m) => {
					try {
						return MS.isTypeSupported(m);
					} catch {
						return false;
					}
				}
			: undefined,
		hdrDisplay: typeof matchMedia === 'function' && matchMedia('(dynamic-range: high)').matches,
		screen: { width: screen.width || innerWidth, height: screen.height || innerHeight, dpr: devicePixelRatio || 1 },
		safari
	};
}

export function detectProfile(quality: QualityChoice = 'auto'): ClientProfile {
	return buildProfile(browserProbes(), quality);
}

/**
 * Fallback ladder after a decode failure: 1 = no direct play (forces a remux),
 * 2 = also H.264/AAC only (forces a transcode of anything else).
 */
export function degradeProfile(profile: ClientProfile, level: number): ClientProfile {
	if (level <= 0) return profile;
	const out: ClientProfile = { ...profile, containers: [] };
	if (level >= 2) {
		out.videoCodecs = profile.videoCodecs.filter((c) => c === 'h264');
		out.audioCodecs = profile.audioCodecs.filter((c) => c === 'aac');
		out.hdr = false;
	}
	return out;
}
