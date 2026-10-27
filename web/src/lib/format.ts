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
