// Web push through Firebase Cloud Messaging: asks permission, registers the
// device token with the API. Returns a message for the user when it cannot.
import { getMessaging, getToken, isSupported } from 'firebase/messaging';
import { firebaseApp } from '$lib/auth.svelte';
import { FIREBASE_VAPID_KEY } from '$lib/config';
import { api } from '$lib/api/client';

export async function enablePush(): Promise<string> {
	if (!('Notification' in window)) return 'This browser does not support notifications.';
	const permission = await Notification.requestPermission();
	if (permission !== 'granted') return 'Notifications are blocked. You can allow them later in your browser settings.';
	const app = firebaseApp();
	if (!app || !FIREBASE_VAPID_KEY || !(await isSupported())) return '';
	try {
		const reg = await navigator.serviceWorker.ready;
		const token = await getToken(getMessaging(app), { vapidKey: FIREBASE_VAPID_KEY, serviceWorkerRegistration: reg });
		if (token) await api.post('/v1/push-tokens', { platform: 'web', token });
		return '';
	} catch {
		return "Couldn't turn on notifications on this device.";
	}
}
