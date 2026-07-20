import { useDeferredValue, useMemo } from "react"

interface SearchEntry<T> {
	item: T
	text: string
}

/**
 * Builds the searchable text only when the source collection changes, then
 * defers query filtering so typing stays responsive for large admin datasets.
 */
export function useSearchFilter<T>(
	items: readonly T[],
	query: string,
	getSearchText: (item: T) => string,
): readonly T[] {
	const deferredQuery = useDeferredValue(query)
	const searchIndex = useMemo<readonly SearchEntry<T>[]>(
		() =>
			items.map((item) => ({
				item,
				text: getSearchText(item).toLowerCase(),
			})),
		[getSearchText, items],
	)

	return useMemo(() => {
		const normalizedQuery = deferredQuery.trim().toLowerCase()
		if (normalizedQuery === "") return items

		return searchIndex
			.filter((entry) => entry.text.includes(normalizedQuery))
			.map((entry) => entry.item)
	}, [deferredQuery, items, searchIndex])
}
