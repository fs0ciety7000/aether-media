// Series title of an episode for cards ("Les Veilleurs · S2 · É4"). The list payloads carry
// only parentId (season), so unknown ones are resolved once through the episode's ancestors.
import { api } from '$lib/api/client';
import type { Item } from '$lib/api/types';

const cache = new Map<number, string>();
const pending = new Map<number, Promise<string | undefined>>();

class SeriesTitles {
	/** episode id → series title (reactive). */
	byEpisode = $state<Record<number, string>>({});

	get(ep: Pick<Item, 'id' | 'seriesTitle'>): string | undefined {
		return ep.seriesTitle ?? this.byEpisode[ep.id];
	}

	/** Resolves the missing titles (at most a dozen per screen). */
	resolve(items: Item[]) {
		for (const ep of items) {
			if (ep.kind !== 'episode' || ep.seriesTitle || this.byEpisode[ep.id] || pending.has(ep.id)) continue;
			const known = cache.get(ep.id);
			if (known) {
				this.byEpisode[ep.id] = known;
				continue;
			}
			const p = api
				.item(ep.id)
				.then((d) => d.ancestors.find((a) => a.kind === 'series')?.title ?? d.ancestors[0]?.title)
				.catch(() => undefined);
			pending.set(ep.id, p);
			p.then((title) => {
				pending.delete(ep.id);
				if (!title) return;
				cache.set(ep.id, title);
				this.byEpisode[ep.id] = title;
			});
		}
	}
}

export const seriesTitles = new SeriesTitles();
