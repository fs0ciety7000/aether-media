// Unit tests of the pure capability detection. Run: npm run test:player (Node ≥ 22, no framework).
import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { buildProfile, degradeProfile, snapHeight } from '../src/lib/player/capabilities.ts';

/** @param {object} over */
function probes(over = {}) {
	const element = over.element ?? /./;
	const mse = over.mse === undefined ? /./ : over.mse;
	return {
		canPlayType: (/** @type {string} */ m) => (element.test(m) ? 'probably' : ''),
		isTypeSupported: mse ? (/** @type {string} */ m) => mse.test(m) : undefined,
		hdrDisplay: false,
		screen: { width: 1920, height: 1080, dpr: 1 },
		safari: false,
		...over
	};
}

// Chromium without proprietary codecs (Playwright's build).
const OPEN = /webm|ogg|flac|wav|vp09|vp9|av01|opus|mpeg"?$|mp4a\.6B|mp4a\.69|^video\/mp4$|^audio\/mp4$|audio\/mpeg/;

test('chromium open-codecs build', () => {
	const p = buildProfile(probes({ element: OPEN, mse: OPEN }));
	assert.deepEqual(p.videoCodecs, ['av1', 'vp9']);
	assert.ok(p.audioCodecs.includes('opus') && p.audioCodecs.includes('flac') && !p.audioCodecs.includes('aac'));
	assert.ok(!p.containers.includes('mkv'));
	assert.equal(p.hls, 'mse');
	assert.equal(p.maxHeight, 1080);
	assert.equal(p.maxBitrate, 0);
	assert.equal(p.hdr, false);
});

test('full chrome with HDR screen', () => {
	const p = buildProfile(probes({ hdrDisplay: true, screen: { width: 1440, height: 900, dpr: 2 } }));
	assert.deepEqual(p.videoCodecs, ['h264', 'hevc', 'av1', 'vp9']);
	assert.ok(['aac', 'ac3', 'eac3'].every((c) => p.audioCodecs.includes(c)));
	assert.equal(p.hdr, true);
	assert.equal(p.maxHeight, 2160);
});

test('safari prefers native HLS; iPhone without MSE too', () => {
	assert.equal(buildProfile(probes({ safari: true })).hls, 'native');
	assert.equal(buildProfile(probes({ mse: null })).hls, 'native');
	assert.equal(buildProfile(probes({ element: /^(?!application)/ })).hls, 'mse');
});

test('quality choice caps height and bitrate', () => {
	const p = buildProfile(probes({ screen: { width: 3840, height: 2160, dpr: 1 } }), '720p');
	assert.equal(p.maxHeight, 720);
	assert.equal(p.maxBitrate, 4_000_000);
	// The screen still wins when smaller than the choice.
	assert.equal(buildProfile(probes({ screen: { width: 390, height: 844, dpr: 1 } }), '1080p').maxHeight, 480);
});

test('ladder and degrade', () => {
	assert.equal(snapHeight(1080), 1080);
	assert.equal(snapHeight(1100), 1440);
	assert.equal(snapHeight(5000), 2160);
	const full = buildProfile(probes());
	assert.deepEqual(degradeProfile(full, 1).containers, []);
	assert.deepEqual(degradeProfile(full, 2).videoCodecs, ['h264']);
	assert.deepEqual(degradeProfile(full, 2).audioCodecs, ['aac']);
});

// ——— Watchdogs (no endless spinner) ———
import { FIRST_FRAME_TIMEOUT_MS, isTimeout, STALL_TIMEOUT_MS, START_TIMEOUT_MS, watchFor } from '../src/lib/player/watchdog.ts';

test('watchdog: waiting for the first image', () => {
	const w = watchFor({ phase: 'playing', firstFrame: false, waiting: false, paused: true });
	assert.equal(w?.ms, FIRST_FRAME_TIMEOUT_MS);
	assert.match(w?.message ?? '', /ne démarre pas/);
});

test('watchdog: stalled after playing, not when paused or idle', () => {
	assert.equal(watchFor({ phase: 'playing', firstFrame: true, waiting: true, paused: false })?.ms, STALL_TIMEOUT_MS);
	assert.equal(watchFor({ phase: 'playing', firstFrame: true, waiting: true, paused: true }), null);
	assert.equal(watchFor({ phase: 'playing', firstFrame: true, waiting: false, paused: false }), null);
	assert.equal(watchFor({ phase: 'error', firstFrame: false, waiting: true, paused: false }), null);
	assert.equal(watchFor({ phase: 'starting', firstFrame: false, waiting: true, paused: false }), null);
});

test('watchdog: start timeout above the server budget', () => {
	assert.ok(START_TIMEOUT_MS > 40_000 + 30_000);
	assert.ok(isTimeout({ name: 'TimeoutError' }));
	assert.ok(!isTimeout(new TypeError('Load failed')));
});
