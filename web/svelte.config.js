import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { loadEnv } from 'vite';

// Origins the app may talk to, from the same VITE_* settings the app reads.
const env = loadEnv(process.env.NODE_ENV === 'development' ? 'development' : 'production', process.cwd(), 'VITE_');
const origin = (url) => {
	try {
		return new URL(url).origin;
	} catch {
		return null;
	}
};
const api = origin(env.VITE_API_URL) ?? 'http://127.0.0.1:8080';
const map = origin(env.VITE_MAP_STYLE_URL) ?? 'https://demotiles.maplibre.org';
const extra = (env.VITE_CSP_CONNECT_SRC ?? '').split(/[\s,]+/).filter(Boolean); // e.g. a separate map tile host
const authDomain = env.VITE_FIREBASE_AUTH_DOMAIN ? [`https://${env.VITE_FIREBASE_AUTH_DOMAIN}`] : [];

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		// A single-page app on Cloudflare Pages: every route falls back to index.html,
		// the API is a separate origin behind the Cloudflare Tunnel.
		adapter: adapter({ fallback: 'index.html' }),
		serviceWorker: { register: true },
		// Written into the page as a <meta> tag with a hash for SvelteKit's own
		// start-up script: no other inline script, and no script from elsewhere
		// (except Google's sign-in helper), can run even if markup is injected.
		csp: {
			mode: 'hash',
			directives: {
				'default-src': ['self'],
				'script-src': ['self', 'https://apis.google.com'],
				// Svelte and MapLibre set style attributes; styles cannot run code.
				'style-src': ['self', 'unsafe-inline', 'https://fonts.googleapis.com'],
				'font-src': ['self', 'https://fonts.gstatic.com'],
				// Company logos come from each company's own site.
				'img-src': ['self', 'data:', 'blob:', 'https:'],
				'connect-src': [
					'self', api, map, ...extra,
					'https://identitytoolkit.googleapis.com', 'https://securetoken.googleapis.com',
					'https://firebaseinstallations.googleapis.com', 'https://fcmregistrations.googleapis.com',
					'https://www.googleapis.com'
				],
				'worker-src': ['self', 'blob:'], // MapLibre draws in a worker
				'child-src': ['self', 'blob:'],
				'frame-src': ['https://*.firebaseapp.com', ...authDomain], // Google / Apple sign-in pop-up helper
				'manifest-src': ['self'],
				'object-src': ['none'],
				'base-uri': ['self'],
				'form-action': ['self']
			}
		}
	}
};
