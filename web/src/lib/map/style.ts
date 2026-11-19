// The globe's map style. VITE_MAP_STYLE_URL wins (Protomaps on R2 later); until
// then OpenFreeMap's free "positron" style is used: a light, quiet base that
// goes from the whole globe down to streets and buildings, with no API key.
// Its data is OpenStreetMap (ODbL), credited by the map's attribution control.
import type { StyleSpecification } from 'maplibre-gl';
import { MAP_STYLE_URL } from '$lib/config';

export const DEFAULT_MAP_STYLE = 'https://tiles.openfreemap.org/styles/positron';

export function mapStyle(): string | StyleSpecification {
	return MAP_STYLE_URL || DEFAULT_MAP_STYLE;
}
