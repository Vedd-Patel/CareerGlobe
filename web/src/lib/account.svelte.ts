// Who the signed-in person is to the API: member, needs a code, removed, …
import { api, ApiError, type Schemas } from '$lib/api/client';

export type AccountState = 'unknown' | 'signed_out' | 'needs_code' | 'removed' | 'unverified' | 'member' | 'error';

export const account = $state<{ state: AccountState; me: Schemas['Me'] | null; error: string }>({
	state: 'unknown',
	me: null,
	error: ''
});

export async function loadMe(): Promise<void> {
	try {
		account.me = await api.get<Schemas['Me']>('/v1/me');
		account.state = 'member';
	} catch (e) {
		account.me = null;
		if (e instanceof ApiError) {
			account.state =
				e.code === 'access_code_required' ? 'needs_code'
				: e.code === 'access_removed' ? 'removed'
				: e.code === 'email_not_verified' ? 'unverified'
				: e.code === 'unauthorized' ? 'signed_out'
				: 'error';
			account.error = e.message;
		} else {
			account.state = 'error';
			account.error = 'Something went wrong.';
		}
	}
}

/** The first onboarding screen still to do, or null when onboarding is done or skipped. */
export function nextOnboarding(me: Schemas['Me'] | null): string | null {
	if (!me?.onboarding) return null;
	let skipped = false;
	try {
		skipped = localStorage.getItem('cg.onboardingSkipped') === '1';
	} catch {
		/* ignore */
	}
	if (!me.onboarding.has_resume && !skipped) return '/onboarding/resume';
	if (me.onboarding.has_resume && !me.onboarding.profile_confirmed) return '/onboarding/review';
	if (!me.onboarding.has_preferences) return '/onboarding/roles';
	return null;
}
