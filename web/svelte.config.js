import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// SPA : le serveur Go renvoie index.html pour toute route inconnue.
		// precompress : copies .br et .gz servies par internal/webui selon Accept-Encoding.
		adapter: adapter({
			pages: '../internal/webui/dist',
			assets: '../internal/webui/dist',
			fallback: 'index.html',
			strict: false,
			precompress: true
		})
	}
};

export default config;
