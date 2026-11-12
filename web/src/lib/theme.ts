// Light / dark / system theme (S29), applied as data-theme on <html>. Light is the default.
export type Theme = 'system' | 'light' | 'dark';
const KEY = 'cg.theme';

export function applyTheme(t: Theme): void {
	if (typeof document === 'undefined') return;
	if (t === 'system') document.documentElement.removeAttribute('data-theme');
	else document.documentElement.setAttribute('data-theme', t);
	try {
		localStorage.setItem(KEY, t);
	} catch {
		/* ignore */
	}
}

export function savedTheme(): Theme {
	try {
		const t = localStorage.getItem(KEY);
		return t === 'dark' || t === 'system' ? t : 'light';
	} catch {
		return 'light';
	}
}
