// Where to go once someone is signed in: the code screen, the removed notice,
// email verification, the next onboarding step, or the globe.
import { goto } from '$app/navigation';
import { account, loadMe, nextOnboarding } from '$lib/account.svelte';
import { api } from '$lib/api/client';

const TICKET = 'cg.ticket';

export function saveTicket(t: string): void {
	try {
		sessionStorage.setItem(TICKET, t);
	} catch {
		/* ignore */
	}
}

export function takeTicket(): string | null {
	try {
		const t = sessionStorage.getItem(TICKET);
		sessionStorage.removeItem(TICKET);
		return t;
	} catch {
		return null;
	}
}

export async function routeAfterSignIn(): Promise<void> {
	await loadMe();
	if (account.state === 'needs_code') {
		const ticket = takeTicket();
		if (ticket) {
			await api.post('/v1/access/redeem', { ticket });
			return routeAfterSignIn();
		}
		return goto('/access');
	}
	if (account.state === 'removed') return goto('/login?removed=1');
	if (account.state === 'unverified') return goto('/verify-email');
	if (account.state === 'member') return goto(nextOnboarding(account.me) ?? '/globe');
}
