/** 132 → "2 h 12" ; 48 → "48 min". */
export function formatDuration(minutes: number): string {
	const h = Math.floor(minutes / 60);
	const m = Math.round(minutes % 60);
	if (!h) return `${m} min`;
	return `${h} h ${String(m).padStart(2, '0')}`;
}

/** Joins metadata parts with a middle dot, skipping empty values. */
export function metaLine(...parts: (string | number | undefined | null | false)[]): string {
	return parts.filter((p) => p !== undefined && p !== null && p !== false && p !== '').join(' · ');
}

/** Player-style clock: 1500 → "25:00" ; 5386 → "1:29:46". */
export function formatClock(totalSeconds: number): string {
	const s = Math.max(0, Math.round(totalSeconds));
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	const sec = String(s % 60).padStart(2, '0');
	return h ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`;
}

/** Runtime in milliseconds → "2 h 12" / "48 min" (undefined when unknown). */
export function formatRuntime(ms: number | undefined | null): string | undefined {
	if (!ms || ms <= 0) return undefined;
	return formatDuration(Math.max(1, Math.round(ms / 60000)));
}

/** Track duration in milliseconds → "3:42". */
export function formatTrackTime(ms: number | undefined | null): string | undefined {
	if (!ms || ms <= 0) return undefined;
	return formatClock(ms / 1000);
}

/** 1 536 000 000 → "1,4 Go". */
export function formatBytes(bytes: number | undefined | null): string | undefined {
	if (!bytes || bytes <= 0) return undefined;
	const units = ['o', 'Ko', 'Mo', 'Go', 'To'];
	let v = bytes;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return `${v.toLocaleString('fr-FR', { maximumFractionDigits: v < 10 && i > 0 ? 1 : 0 })} ${units[i]}`;
}

/** 8 400 000 → "8,4 Mb/s". */
export function formatBitrate(bps: number | undefined | null): string | undefined {
	if (!bps || bps <= 0) return undefined;
	if (bps >= 1_000_000) return `${(bps / 1_000_000).toLocaleString('fr-FR', { maximumFractionDigits: 1 })} Mb/s`;
	return `${Math.round(bps / 1000)} kb/s`;
}

/** "2024-03-07" → "7 mars 2024". */
export function formatDate(iso: string | undefined | null): string | undefined {
	if (!iso) return undefined;
	const d = new Date(iso.length === 10 ? `${iso}T12:00:00` : iso);
	if (Number.isNaN(d.getTime())) return iso;
	return d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
}

/** 1 → "1 film", 3 → "3 films". */
export function plural(n: number, one: string, many: string): string {
	return `${n.toLocaleString('fr-FR')} ${n > 1 ? many : one}`;
}

const RTF = typeof Intl !== 'undefined' ? new Intl.RelativeTimeFormat('fr-FR', { numeric: 'auto' }) : null;

/** "il y a 5 minutes", "hier"… (undefined when the date is missing or invalid). */
export function formatRelative(iso: string | undefined | null, now = Date.now()): string | undefined {
	if (!iso) return undefined;
	const t = new Date(iso).getTime();
	if (Number.isNaN(t)) return undefined;
	const sec = Math.round((t - now) / 1000);
	const abs = Math.abs(sec);
	if (abs < 45) return 'à l’instant';
	if (!RTF) return formatDate(iso);
	if (abs < 3600) return RTF.format(Math.round(sec / 60), 'minute');
	if (abs < 86400) return RTF.format(Math.round(sec / 3600), 'hour');
	if (abs < 86400 * 30) return RTF.format(Math.round(sec / 86400), 'day');
	return formatDate(iso);
}
