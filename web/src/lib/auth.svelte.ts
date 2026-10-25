// Sign-in: Firebase Auth (Google, Apple, email + password), or in local
// development a fake account the API accepts only with DEV_AUTH=1 on localhost.
import { initializeApp, type FirebaseApp } from 'firebase/app';
import {
	getAuth,
	onAuthStateChanged,
	signInWithPopup,
	GoogleAuthProvider,
	OAuthProvider,
	createUserWithEmailAndPassword,
	signInWithEmailAndPassword,
	sendEmailVerification,
	sendPasswordResetEmail,
	signOut as fbSignOut,
	type Auth,
	type User
} from 'firebase/auth';
import { DEV_AUTH, FIREBASE, firebaseConfigured } from '$lib/config';
import { setTokenSource } from '$lib/api/client';

export type SessionUser = { uid: string; email: string | null; name: string | null; emailVerified: boolean };

export const session = $state<{ ready: boolean; user: SessionUser | null }>({ ready: false, user: null });

let app: FirebaseApp | null = null;
let auth: Auth | null = null;
const DEV_KEY = 'cg.devUser';

export function firebaseApp(): FirebaseApp | null {
	return app;
}

export function initAuth(): void {
	if (typeof window === 'undefined' || session.ready) return;
	if (firebaseConfigured()) {
		app = initializeApp(FIREBASE as Record<string, string>);
		auth = getAuth(app);
		setTokenSource(async () => (auth?.currentUser ? auth.currentUser.getIdToken() : null));
		onAuthStateChanged(auth, (u: User | null) => {
			session.user = u ? { uid: u.uid, email: u.email, name: u.displayName, emailVerified: u.emailVerified } : null;
			session.ready = true;
		});
		return;
	}
	// No Firebase project yet: local development sign-in only.
	setTokenSource(async () => {
		const u = devUser();
		return DEV_AUTH && u ? `dev:${u.uid}:${u.email ?? ''}` : null;
	});
	session.user = DEV_AUTH ? devUser() : null;
	session.ready = true;
}

function devUser(): SessionUser | null {
	try {
		const raw = localStorage.getItem(DEV_KEY);
		return raw ? (JSON.parse(raw) as SessionUser) : null;
	} catch {
		return null;
	}
}

function devSignIn(email: string): void {
	const uid = email.split('@')[0].replace(/[^a-z0-9]/gi, '').toLowerCase() || 'me';
	const u = { uid, email, name: uid, emailVerified: true };
	try {
		localStorage.setItem(DEV_KEY, JSON.stringify(u));
	} catch {
		/* private mode: session only */
	}
	session.user = u;
}

export const authAvailable = (): boolean => firebaseConfigured() || DEV_AUTH;

export async function signInGoogle(): Promise<void> {
	if (!auth) return devSignIn('dev.google@example.com');
	await signInWithPopup(auth, new GoogleAuthProvider());
}

export async function signInApple(): Promise<void> {
	if (!auth) return devSignIn('dev.apple@example.com');
	await signInWithPopup(auth, new OAuthProvider('apple.com'));
}

export async function signUpEmail(email: string, password: string): Promise<void> {
	if (!auth) return devSignIn(email);
	const cred = await createUserWithEmailAndPassword(auth, email, password);
	await sendEmailVerification(cred.user);
}

export async function signInEmail(email: string, password: string): Promise<void> {
	if (!auth) return devSignIn(email);
	await signInWithEmailAndPassword(auth, email, password);
}

export async function resendVerification(): Promise<void> {
	if (auth?.currentUser) await sendEmailVerification(auth.currentUser);
}

export async function reloadUser(): Promise<void> {
	if (!auth?.currentUser) return;
	await auth.currentUser.reload();
	await auth.currentUser.getIdToken(true);
	const u = auth.currentUser;
	session.user = { uid: u.uid, email: u.email, name: u.displayName, emailVerified: u.emailVerified };
}

export async function resetPassword(email: string): Promise<void> {
	if (auth) await sendPasswordResetEmail(auth, email);
}

export async function signOut(): Promise<void> {
	if (auth) await fbSignOut(auth);
	try {
		localStorage.removeItem(DEV_KEY);
	} catch {
		/* ignore */
	}
	session.user = null;
}

/** Firebase error codes in words. */
export function authMessage(e: unknown): string {
	const code = (e as { code?: string })?.code ?? '';
	const map: Record<string, string> = {
		'auth/invalid-credential': 'Email or password is wrong.',
		'auth/wrong-password': 'Email or password is wrong.',
		'auth/user-not-found': 'No account with this email.',
		'auth/email-already-in-use': 'An account with this email already exists. Log in instead.',
		'auth/weak-password': 'Use a password of at least 8 characters.',
		'auth/invalid-email': 'That email address is not valid.',
		'auth/popup-closed-by-user': 'Sign-in was cancelled.',
		'auth/too-many-requests': 'Too many tries. Wait a minute and try again.',
		'auth/network-request-failed': "You're offline. Check your connection."
	};
	return map[code] ?? (e instanceof Error ? e.message : 'Sign-in failed.');
}
