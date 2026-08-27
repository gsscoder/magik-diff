import {diffparse} from "../wailsjs/go/models";

export interface SplitRow {
    left: diffparse.Line | null;
    right: diffparse.Line | null;
}

// Mirrors the grouping rule of addIntralineHighlights in
// internal/diffparse/diffparse.go: a maximal run of removed lines
// immediately followed by a maximal run of added lines forms one group,
// paired by position within the group.
export function splitRows(lines: diffparse.Line[]): SplitRow[] {
    const rows: SplitRow[] = [];
    let i = 0;
    while (i < lines.length) {
        const line = lines[i];
        if (line.Type === "context") {
            rows.push({left: line, right: line});
            i++;
            continue;
        }
        if (line.Type === "removed") {
            const rStart = i;
            while (i < lines.length && lines[i].Type === "removed") {
                i++;
            }
            const aStart = i;
            while (i < lines.length && lines[i].Type === "added") {
                i++;
            }
            const removed = lines.slice(rStart, aStart);
            const added = lines.slice(aStart, i);
            const count = Math.max(removed.length, added.length);
            for (let k = 0; k < count; k++) {
                rows.push({
                    left: k < removed.length ? removed[k] : null,
                    right: k < added.length ? added[k] : null,
                });
            }
            continue;
        }
        // Added line with no preceding removed run: pure insertion.
        rows.push({left: null, right: line});
        i++;
    }
    return rows;
}
