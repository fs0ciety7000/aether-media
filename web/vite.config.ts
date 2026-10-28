import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// `make dev-api` serves the API on :8096; AETHER_API points the proxy elsewhere.
const env = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env ?? {};
const api = env.AETHER_API ?? 'http://127.0.0.1:8096';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: { '/api': { target: api, changeOrigin: true } }
	}
});
