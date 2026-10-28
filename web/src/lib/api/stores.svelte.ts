// App-wide reactive data: libraries, server info, live scans, items to identify.
// Each store loads lazily and keeps itself fresh from server events.
import { api } from './client';
import { events } from './events.svelte';
import type { Kind, Library, ScanProgress, ServerEvent, Source, SourceStatusEvent, SystemInfo } from './types';

class LibrariesStore {
	list = $state<Library[]>([]);
	loaded = $state(false);
	error = $state<unknown>(null);
	#started = false;
	#pending: Promise<void> | null = null;

	/** Loads once (idempotent); subsequent calls return the same promise. */
	ensure(): Promise<void> {
		if (!this.#started) {
			this.#started = true;
			events.debounced(['library.scan.finished'], () => this.refresh(), 600);
			events.on('source.status', (ev) => this.applySourceStatus(ev.data as SourceStatusEvent));
		}
		return this.#pending ?? this.refresh();
	}

	refresh(): Promise<void> {
		const p = api
			.libraries()
			.then((libs) => {
				this.list = libs ?? [];
				this.error = null;
			})
			.catch((err) => {
				this.error = err;
			})
			.finally(() => {
				this.loaded = true;
				if (this.#pending === p) this.#pending = null;
			});
		this.#pending = p;
		return p;
	}

	get(id: number): Library | undefined {
		return this.list.find((l) => l.id === id);
	}

	/** Live `source.status` event: patches the source in place (no refetch). */
	applySourceStatus(d: SourceStatusEvent | undefined) {
		if (!d || typeof d.id !== 'number') return;
		const lib = this.list.find((l) => l.id === d.libraryId) ?? this.list.find((l) => l.sources.some((s) => s.id === d.id));
		const src = lib?.sources.find((s) => s.id === d.id);
		if (!src) {
			// A source we do not know yet (added elsewhere): reload quietly.
			if (this.loaded) this.refresh();
			return;
		}
		src.status = d.status ?? 'unknown';
		src.lastError = d.lastError;
		if (src.status === 'online') src.lastSeen = new Date().toISOString();
	}

	/** Local updates after a successful POST / PATCH / DELETE. */
	upsertSource(libraryId: number, src: Source) {
		const lib = this.get(libraryId);
		if (!lib) return;
		const i = lib.sources.findIndex((s) => s.id === src.id);
		if (i >= 0) lib.sources[i] = src;
		else lib.sources.push(src);
	}

	removeSource(libraryId: number, sourceId: number) {
		const lib = this.get(libraryId);
		if (lib) lib.sources = lib.sources.filter((s) => s.id !== sourceId);
	}
}

export const libraries = new LibrariesStore();

class SystemStore {
	info = $state<SystemInfo | null>(null);
	#started = false;

	ensure() {
		if (this.#started) return;
		this.#started = true;
		api
			.systemInfo()
			.then((i) => (this.info = i))
			.catch(() => {});
	}
}

export const system = new SystemStore();

export interface ScanState {
	running: boolean;
	processed: number;
	total: number;
	/** Set when the last scan finished. */
	finishedAt?: number;
	added?: number;
}

/** Live scan progress per library, fed by library.scan.* events. */
class ScansStore {
	byLibrary = $state<Record<number, ScanState>>({});
	#started = false;

	ensure() {
		if (this.#started) return;
		this.#started = true;
		events.on(['library.scan.started', 'library.scan.progress', 'library.scan.finished'], (ev) => this.#apply(ev));
	}

	/** Marks a library as queued right after POST /scan, before the first event. */
	queued(id: number) {
		this.byLibrary[id] = { running: true, processed: 0, total: 0 };
	}

	#apply(ev: ServerEvent) {
		const d = (ev.data ?? {}) as Partial<ScanProgress> & { added?: number };
		const id = d.libraryId;
		if (typeof id !== 'number') return;
		const cur = this.byLibrary[id] ?? { running: true, processed: 0, total: 0 };
		if (ev.type === 'library.scan.started') this.byLibrary[id] = { running: true, processed: 0, total: 0 };
		else if (ev.type === 'library.scan.progress')
			this.byLibrary[id] = { ...cur, running: true, processed: d.processed ?? cur.processed, total: d.total ?? cur.total };
		else if (ev.type === 'library.scan.finished')
			this.byLibrary[id] = { ...cur, running: false, processed: cur.total, finishedAt: Date.now(), added: d.added };
	}
}

export const scans = new ScansStore();

export const IDENTIFIABLE_KINDS: Kind[] = ['movie', 'series', 'artist', 'album'];

/** Number of items waiting for a manual identification (sidebar badge). */
class UnmatchedStore {
	count = $state(0);
	#started = false;

	ensure() {
		if (this.#started) return;
		this.#started = true;
		this.refresh();
		events.debounced(['item.updated', 'library.scan.finished'], () => this.refresh(), 1500);
	}

	refresh() {
		api
			.items({ identify: 'unmatched', kind: IDENTIFIABLE_KINDS, limit: 1 })
			.then((p) => (this.count = p.total ?? p.items.length))
			.catch(() => {});
	}
}

export const unmatched = new UnmatchedStore();
