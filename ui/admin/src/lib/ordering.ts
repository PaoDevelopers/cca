// Moving one entry within a display order.
//
// The server takes the whole order, not a relative move: it names the
// arrangement the administrator is looking at, so the write is
// idempotent and a double-click cannot reorder twice. The up/down
// buttons stay as the gesture — they are what a small list wants —
// but each one computes the resulting order and sends that.

export function reordered<T extends string>(
	ids: readonly T[],
	id: T,
	direction: "up" | "down",
): T[] {
	const from = ids.indexOf(id)
	const to = direction === "up" ? from - 1 : from + 1
	if (from === -1 || to < 0 || to >= ids.length) {
		// Already at the edge: the order is unchanged, and sending it
		// is a no-op the server accepts happily.
		return [...ids]
	}
	const out = [...ids]
	;[out[from], out[to]] = [out[to] as T, out[from] as T]
	return out
}
