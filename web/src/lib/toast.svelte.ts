// Minimal toast queue. Toasts are quiet: neutral surface, no coloured icon.

export interface ToastAction {
	label: string;
	run: () => void;
}

export interface Toast {
	id: number;
	message: string;
	detail?: string;
	action?: ToastAction;
}

interface ToastOptions {
	detail?: string;
	action?: ToastAction;
	/** Milliseconds before auto-dismiss; 0 keeps it until closed. */
	duration?: number;
}

const MAX_VISIBLE = 3;
let nextId = 1;
const timers = new Map<number, ReturnType<typeof setTimeout>>();

class ToastStore {
	items = $state<Toast[]>([]);

	show(message: string, { detail, action, duration = 5000 }: ToastOptions = {}) {
		const id = nextId++;
		this.items = [...this.items, { id, message, detail, action }].slice(-MAX_VISIBLE);
		if (duration > 0) timers.set(id, setTimeout(() => this.dismiss(id), duration));
		return id;
	}

	dismiss(id: number) {
		clearTimeout(timers.get(id));
		timers.delete(id);
		this.items = this.items.filter((t) => t.id !== id);
	}
}

export const toasts = new ToastStore();
