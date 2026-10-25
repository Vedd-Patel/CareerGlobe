// The client-side mirror of the API's checks (the API is what actually enforces them).
import { goto } from '$app/navigation';
import { account, loadMe } from '$lib/account.svelte';
import { session } from '$lib/auth.svelte';

/** Resolves true when the signed-in user is a member; otherwise sends them where they belong. */
export async function requireMember(): Promise<boolean> {
	if (!session.user) {
		goto('/login');
		return false;
	}
	if (account.state !== 'member') await loadMe();
	switch (account.state) {
		case 'member':
			return true;
		case 'needs_code':
			goto('/access');
			return false;
		case 'removed':
			goto('/login?removed=1');
			return false;
		case 'unverified':
			goto('/verify-email');
			return false;
		case 'signed_out':
			goto('/login');
			return false;
		default:
			return false;
	}
}
