import { describe, expect, it } from 'vitest';
import { clusterPoints, compact, type ClusterPoint } from './cluster';

const pt = (key: string, x: number, y: number, open = 1, matches = 0): ClusterPoint => ({ key, x, y, open, matches });

describe('clusterPoints', () => {
	it('keeps far pins apart and merges near ones', () => {
		const g = clusterPoints([pt('a', 0, 0), pt('b', 10, 0), pt('c', 300, 300)], 40);
		expect(g.map((x) => x.members.sort())).toEqual([['a', 'b'], ['c']]);
	});
	it('lets the pin with most matches lead, so the bubble does not jump', () => {
		const g = clusterPoints([pt('a', 0, 0, 5, 0), pt('b', 10, 0, 1, 3)], 40);
		expect(g).toEqual([{ seed: 'b', members: ['b', 'a'] }]);
	});
	it('puts every pin in exactly one group', () => {
		const pts = Array.from({ length: 60 }, (_, i) => pt(`p${i}`, (i * 37) % 200, (i * 91) % 200));
		const all = clusterPoints(pts, 30).flatMap((g) => g.members);
		expect(all.sort()).toEqual(pts.map((p) => p.key).sort());
	});
	it('merging more as the radius grows', () => {
		const pts = [pt('a', 0, 0), pt('b', 50, 0), pt('c', 100, 0)];
		expect(clusterPoints(pts, 20)).toHaveLength(3);
		expect(clusterPoints(pts, 120)).toHaveLength(1);
	});
});

describe('compact', () => {
	it('shortens big counts', () => {
		expect([compact(7), compact(999), compact(1000), compact(1240), compact(12400)]).toEqual(['7', '999', '1k', '1.2k', '12k']);
	});
});
