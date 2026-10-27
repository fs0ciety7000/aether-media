import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		// `make dev-api` sert l'API sur :8096.
		proxy: { '/api': 'http://127.0.0.1:8096' }
	}
});
