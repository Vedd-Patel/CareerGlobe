// The globe's map style. VITE_MAP_STYLE_URL (Protomaps on R2) wins; until it
// exists, MapLibre's free demo world tiles are drawn in the CareerGlobe palette
// (globe.ocean / land / landFocus / border in docs/designs/tokens.json).
import type { StyleSpecification } from 'maplibre-gl';
import { MAP_STYLE_URL } from '$lib/config';

export function mapStyle(): string | StyleSpecification {
	if (MAP_STYLE_URL) return MAP_STYLE_URL;
	return {
		version: 8,
		sources: {
			world: { type: 'vector', url: 'https://demotiles.maplibre.org/tiles/tiles.json', attribution: '© MapLibre · Natural Earth' }
		},
		layers: [
			{ id: 'ocean', type: 'background', paint: { 'background-color': '#D3E6EE' } },
			{ id: 'land', type: 'fill', source: 'world', 'source-layer': 'countries', paint: { 'fill-color': '#FAF9F5' } },
			{
				id: 'india', type: 'fill', source: 'world', 'source-layer': 'countries',
				filter: ['==', ['get', 'ADM0_A3'], 'IND'], paint: { 'fill-color': '#E4F1E3' }
			},
			{ id: 'borders', type: 'line', source: 'world', 'source-layer': 'countries', paint: { 'line-color': '#C7CEC4', 'line-width': 0.8 } }
		]
	};
}
