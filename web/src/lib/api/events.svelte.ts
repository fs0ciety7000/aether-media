// Server-sent events (/api/v1/events) shared by the whole app.
// One EventSource, opened on first subscription, reconnecting with backoff.
import { API_BASE } from './client';
import type { ServerEvent } from './types';

export const EVENT_TYPES = [
	'library.scan.started',
	'library.scan.progress',
	'library.scan.finished',
	'item.updated',
	'item.image',
	'source.status'
] as const;

export type EventType = (typeof EVENT_TYPES)[number];
type Handler = (ev: ServerEvent) => void;

class EventStream {
	/** connecting · open · closed (waiting before a retry) */
	status = $state<'idle' | 'connecting' | 'open' | 'closed'>('idle');

	#source: EventSource | null = null;
	#handlers = new Map<string, Set<Handler>>();
	#retry = 0;
	#timer: ReturnType<typeof setTimeout> | undefined;

	/** Subscribes to event types ('*' for all). Returns the unsubscribe function. */
	on(types: EventType | EventType[] | '*', handler: Handler): () => void {
		const list = types === '*' ? ['*'] : Array.isArray(types) ? types : [types];
		for (const t of list) {
			if (!this.#handlers.has(t)) this.#handlers.set(t, new Set());
			this.#handlers.get(t)!.add(handler);
		}
		this.#connect();
		return () => {
			for (const t of list) this.#handlers.get(t)?.delete(handler);
		};
	}

	/**
	 * Calls `fn` once things calm down after a burst of matching events
	 * (e.g. refresh a list on item.updated). Returns the unsubscribe function.
	 */
	debounced(types: EventType | EventType[], fn: (last: ServerEvent) => void, wait = 800, filter?: (ev: ServerEvent) => boolean): () => void {
		let timer: ReturnType<typeof setTimeout> | undefined;
		const off = this.on(types, (ev) => {
			if (filter && !filter(ev)) return;
			clearTimeout(timer);
			timer = setTimeout(() => fn(ev), wait);
		});
		return () => {
			clearTimeout(timer);
			off();
		};
	}

	#emit(ev: ServerEvent) {
		for (const key of [ev.type, '*']) {
			for (const h of this.#handlers.get(key) ?? []) {
				try {
					h(ev);
				} catch (err) {
					console.error('event handler', err);
				}
			}
		}
	}

	#connect() {
		if (this.#source || this.#timer || typeof EventSource === 'undefined') return;
		this.status = 'connecting';
		const src = new EventSource(`${API_BASE}/events`);
		this.#source = src;
		src.onopen = () => {
			this.status = 'open';
			this.#retry = 0;
		};
		const onMessage = (e: MessageEvent) => {
			try {
				const ev = JSON.parse(e.data) as ServerEvent;
				if (!ev.type) ev.type = e.type;
				this.#emit(ev);
			} catch {
				/* ignore malformed payloads */
			}
		};
		for (const t of EVENT_TYPES) src.addEventListener(t, onMessage);
		src.onmessage = onMessage;
		src.onerror = () => {
			// The browser retries by itself while CONNECTING; once CLOSED it gives up: we take over.
			if (src.readyState !== EventSource.CLOSED) {
				this.status = 'connecting';
				return;
			}
			src.close();
			this.#source = null;
			this.status = 'closed';
			const delay = Math.min(30_000, 1000 * 2 ** this.#retry++);
			this.#timer = setTimeout(() => {
				this.#timer = undefined;
				this.#connect();
			}, delay);
		};
	}
}

export const events = new EventStream();
