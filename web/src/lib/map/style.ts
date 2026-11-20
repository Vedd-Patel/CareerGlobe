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

// ---- A lighter dark map ----
// OpenFreeMap's dark style is nearly black. This reads it and lifts every fill, line and
// background colour into a warm grey-brown, keeping water bluish, so land, water and roads
// all stay easy to tell apart. Label text colours are left alone.

const COLOR_KEYS = new Set([
	'background-color', 'fill-color', 'fill-outline-color', 'line-color', 'fill-extrusion-color', 'circle-color', 'text-halo-color'
]);

/** "#abc", "rgb(...)", "hsl(...)" and their alpha forms -> [h 0-360, s 0-1, l 0-1, a], or null. */
export function parseColor(input: string): [number, number, number, number] | null {
	const v = input.trim().toLowerCase();
	let r = 0, g = 0, b = 0, a = 1;
	let m = v.match(/^#([0-9a-f]{3,8})$/);
	if (m) {
		let h = m[1];
		if (h.length === 3 || h.length === 4) h = [...h].map((c) => c + c).join('');
		if (h.length !== 6 && h.length !== 8) return null;
		[r, g, b] = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
		if (h.length === 8) a = parseInt(h.slice(6, 8), 16) / 255;
	} else if ((m = v.match(/^rgba?\(([^)]+)\)$/))) {
		const n = m[1].split(/[\s,/]+/).filter(Boolean).map((x) => (x.endsWith('%') ? (parseFloat(x) / 100) * 255 : parseFloat(x)));
		if (n.length < 3 || n.some(Number.isNaN)) return null;
		[r, g, b] = [n[0] / 255, n[1] / 255, n[2] / 255];
		if (n.length > 3) a = n[3] > 1 ? n[3] / 255 : n[3];
	} else if ((m = v.match(/^hsla?\(([^)]+)\)$/))) {
		const n = m[1].split(/[\s,/]+/).filter(Boolean).map((x) => parseFloat(x));
		if (n.length < 3 || n.some(Number.isNaN)) return null;
		return [((n[0] % 360) + 360) % 360, n[1] / 100, n[2] / 100, n.length > 3 ? n[3] : 1];
	} else return null;
	const max = Math.max(r, g, b), min = Math.min(r, g, b), l = (max + min) / 2, d = max - min;
	if (d === 0) return [0, 0, l, a];
	const sat = d / (1 - Math.abs(2 * l - 1));
	const hue = max === r ? ((g - b) / d + (g < b ? 6 : 0)) : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
	return [hue * 60, sat, l, a];
}

/** Lift one colour: neutrals become a warm grey, tinted colours keep their hue; everything gets lighter. */
export function liftColor(input: string, boost = 0): string {
	const c = parseColor(input);
	if (!c) return input;
	const [h, s, l, a] = c;
	const neutral = s < 0.2;
	const hue = neutral ? 32 : h;
	const sat = neutral ? 0.07 : s * 0.8;
	const light = Math.min(0.9, (neutral ? 0.2 : 0.22) + l * 0.55 + boost);
	return `hsla(${hue.toFixed(0)}, ${(sat * 100).toFixed(0)}%, ${(light * 100).toFixed(1)}%, ${a})`;
}

function liftValue(v: unknown, boost = 0): unknown {
	if (typeof v === 'string') return liftColor(v, boost);
	if (Array.isArray(v)) return v.map((x, i) => (i === 0 && typeof x === 'string' ? x : liftValue(x, boost)));
	if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, k === 'stops' ? liftStops(x, boost) : liftValue(x, boost)]));
	return v;
}

function liftStops(stops: unknown, boost = 0): unknown {
	return Array.isArray(stops) ? stops.map((s) => (Array.isArray(s) ? [s[0], liftValue(s[1], boost)] : s)) : stops;
}

// Fixed colours for the features that must read apart at a glance: land, water, parks,
// buildings and each road class. Anything not listed keeps its lifted colour.
const DARK_PAINT: Record<string, Record<string, string>> = {
	background: { 'background-color': '#3a3632' },
	landuse_residential: { 'fill-color': '#413c37' },
	landcover_wood: { 'fill-color': '#35493a' },
	landuse_park: { 'fill-color': '#3b5640' },
	water: { 'fill-color': '#2c5f8a' },
	waterway: { 'line-color': '#3a77a8' },
	building: { 'fill-color': '#4d4741' },
	highway_path: { 'line-color': '#6d655c' },
	highway_minor: { 'line-color': '#7b736a' },
	highway_major_casing: { 'line-color': '#2b2825' },
	highway_major_inner: { 'line-color': '#a39a8e' },
	highway_major_subtle: { 'line-color': '#8c8478' },
	highway_motorway_casing: { 'line-color': '#2b2825' },
	highway_motorway_inner: { 'line-color': '#d9a35b' },
	highway_motorway_subtle: { 'line-color': '#9a7a4a' },
	railway: { 'line-color': '#6a6259' },
	boundary_state: { 'line-color': '#8a8176' },
	'boundary_country_z0-4': { 'line-color': '#a39a8e' },
	'boundary_country_z5-': { 'line-color': '#a39a8e' }
};

const LABEL_TEXT = '#d8d2c8';

/** The style with every fill, line and background colour lifted (see above). */
export function liftStyle(style: StyleSpecification): StyleSpecification {
	return {
		...style,
		layers: style.layers.map((layer) => {
			const paint = (layer as { paint?: Record<string, unknown> }).paint;
			if (!paint) return layer;
			const next: Record<string, unknown> = { ...paint };
			// Lines (roads, borders) get an extra lift so they stand out from the land.
			for (const k of Object.keys(paint)) if (COLOR_KEYS.has(k)) next[k] = liftValue(paint[k], k === 'line-color' ? 0.1 : 0);
			return { ...layer, paint: next } as typeof layer;
		})
	};
}

/** The map style for the current theme; the dark one is fetched and lifted (falls back to the plain URL). */
export async function themedStyle(dark: boolean): Promise<string | StyleSpecification> {
	if (!dark || MAP_STYLE_URL) return mapStyle(dark);
	try {
		const res = await fetch(DARK_MAP_STYLE);
		if (!res.ok) throw new Error(String(res.status));
		return liftStyle((await res.json()) as StyleSpecification);
	} catch {
		return mapStyle(true);
	}
}
