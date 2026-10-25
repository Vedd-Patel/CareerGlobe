// The globe's map style. VITE_MAP_STYLE_URL (Protomaps on R2) wins; until it
// exists, MapLibre's free demo world tiles are drawn in the CareerGlobe palette.
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
			{ id: 'ocean', type: 'background', paint: { 'background-color': '#0A1530' } },
			{ id: 'land', type: 'fill', source: 'world', 'source-layer': 'countries', paint: { 'fill-color': '#1C2C52' } },
			{
				id: 'india', type: 'fill', source: 'world', 'source-layer': 'countries',
				filter: ['==', ['get', 'ADM0_A3'], 'IND'], paint: { 'fill-color': '#22386A' }
			},
			{ id: 'borders', type: 'line', source: 'world', 'source-layer': 'countries', paint: { 'line-color': '#2E4372', 'line-width': 0.8 } }
		]
	};
}
