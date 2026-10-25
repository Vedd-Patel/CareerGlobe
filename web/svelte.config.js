import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		// A single-page app on Cloudflare Pages: every route falls back to index.html,
		// the API is a separate origin behind the Cloudflare Tunnel.
		adapter: adapter({ fallback: 'index.html' }),
		serviceWorker: { register: true }
	}
};
