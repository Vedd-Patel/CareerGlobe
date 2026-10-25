// Preferences edited across S10-S13 and S28/S29, saved with one PUT.
import { api, type Schemas } from '$lib/api/client';

export const prefs = $state<{ value: Schemas['Preferences'] | null }>({ value: null });

export async function loadPrefs(): Promise<Schemas['Preferences']> {
	if (!prefs.value) prefs.value = await api.get<Schemas['Preferences']>('/v1/preferences');
	return prefs.value;
}

export async function savePrefs(patch: Partial<Schemas['Preferences']>): Promise<void> {
	const cur = await loadPrefs();
	const next = { ...cur, ...patch };
	const { saved: _saved, ...body } = next;
	prefs.value = await api.put<Schemas['Preferences']>('/v1/preferences', body);
}
