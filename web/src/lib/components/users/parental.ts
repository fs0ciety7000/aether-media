import type { RatingAge, User } from '$lib/api/types';

export const RATING_OPTIONS: { value: string; label: string }[] = [
	{ value: '', label: 'Aucun' },
	{ value: '0', label: 'Tous publics (0)' },
	{ value: '6', label: '6 ans' },
	{ value: '10', label: '10 ans' },
	{ value: '12', label: '12 ans' },
	{ value: '16', label: '16 ans' },
	{ value: '18', label: '18 ans' }
];

export function parentalLabel(u: Pick<User, 'maxRatingAge' | 'blockUnrated'>): string {
	if (u.maxRatingAge === undefined || u.maxRatingAge === null) return u.blockUnrated ? 'Non classés masqués' : 'Aucun';
	const base = u.maxRatingAge === 0 ? 'Tous publics' : `${u.maxRatingAge} ans max.`;
	return u.blockUnrated ? `${base} · non classés masqués` : base;
}

export function toAge(v: string): RatingAge | null {
	return v === '' ? null : (Number(v) as RatingAge);
}
