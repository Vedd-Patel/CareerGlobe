// Screen-space clustering for the globe: pins closer than `radius` pixels share
// one bubble. It runs on every map frame, so bubbles merge into their totals
// as you zoom out and split as you zoom in (like stops on a transit map).

export type ClusterPoint = { key: string; x: number; y: number; open: number; matches: number };
export type Group = { seed: string; members: string[] };

/**
 * Greedy grouping. Points are visited best first (most matches, then most open
 * jobs, then key), so the same pin keeps leading its bubble from frame to frame
 * and bubbles do not jump around while you zoom.
 */
export function clusterPoints(points: ClusterPoint[], radius: number): Group[] {
	const order = [...points].sort((a, b) => b.matches - a.matches || b.open - a.open || (a.key < b.key ? -1 : 1));
	const taken = new Set<string>();
	const r2 = radius * radius;
	const groups: Group[] = [];
	for (const seed of order) {
		if (taken.has(seed.key)) continue;
		taken.add(seed.key);
		const members = [seed.key];
		for (const p of order) {
			if (taken.has(p.key)) continue;
			const dx = p.x - seed.x, dy = p.y - seed.y;
			if (dx * dx + dy * dy <= r2) {
				taken.add(p.key);
				members.push(p.key);
			}
		}
		groups.push({ seed: seed.key, members });
	}
	return groups;
}

/** 950 -> "950", 1240 -> "1.2k", 12400 -> "12k". */
export function compact(n: number): string {
	if (n < 1000) return String(n);
	return n < 10000 ? `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k` : `${Math.round(n / 1000)}k`;
}
