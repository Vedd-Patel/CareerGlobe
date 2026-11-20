// The globe's map style. VITE_MAP_STYLE_URL wins (Protomaps on R2 later); until
// then OpenFreeMap's free styles are used: "positron" (light, quiet) and "dark",
// both from the whole globe down to streets and buildings, with no API key.
// Their data is OpenStreetMap (ODbL), credited by the map's attribution control.
import type { StyleSpecification } from 'maplibre-gl';
import { MAP_STYLE_URL } from '$lib/config';

export const DEFAULT_MAP_STYLE = 'https://tiles.openfreemap.org/styles/positron';
export const DARK_MAP_STYLE = 'https://tiles.openfreemap.org/styles/dark';

export function mapStyle(dark = false): string | StyleSpecification {
	return MAP_STYLE_URL || (dark ? DARK_MAP_STYLE : DEFAULT_MAP_STYLE);
}

/** True when the page is showing the dark theme (chosen, or "system" with a dark OS). */
export function isDarkTheme(): boolean {
	if (typeof document === 'undefined') return false;
	const t = document.documentElement.getAttribute('data-theme');
	return t === 'dark' || (t !== 'light' && matchMedia('(prefers-color-scheme: dark)').matches);
}
