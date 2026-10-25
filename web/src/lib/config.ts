// Public build-time settings (VITE_*): nothing secret ships in the browser.
const env = import.meta.env;

export const API_URL: string = (env.VITE_API_URL as string | undefined)?.replace(/\/$/, '') || 'http://127.0.0.1:8080';
export const MAP_STYLE_URL: string = (env.VITE_MAP_STYLE_URL as string | undefined) || '';
export const DEV_AUTH: boolean = env.VITE_DEV_AUTH === '1';
export const FIREBASE = {
	apiKey: env.VITE_FIREBASE_API_KEY as string | undefined,
	authDomain: env.VITE_FIREBASE_AUTH_DOMAIN as string | undefined,
	projectId: env.VITE_FIREBASE_PROJECT_ID as string | undefined,
	appId: env.VITE_FIREBASE_APP_ID as string | undefined,
	messagingSenderId: env.VITE_FIREBASE_MESSAGING_SENDER_ID as string | undefined
};
export const FIREBASE_VAPID_KEY: string = (env.VITE_FIREBASE_VAPID_KEY as string | undefined) || '';
export const firebaseConfigured = (): boolean => Boolean(FIREBASE.apiKey && FIREBASE.projectId);
