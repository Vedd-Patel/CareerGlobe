// The six first-run steps, in order (S08-S13).
export const ONBOARDING = ['/onboarding/resume', '/onboarding/review', '/onboarding/roles', '/onboarding/locations', '/onboarding/level', '/onboarding/notifications'];

export function nextStep(current: string): string {
	const i = ONBOARDING.indexOf(current);
	return i >= 0 && i < ONBOARDING.length - 1 ? ONBOARDING[i + 1] : '/globe';
}
