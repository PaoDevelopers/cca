export const CCA_DAYS = ["Monday", "Tuesday", "Wednesday", "Thursday"] as const

export const CCA_SLOTS_PER_DAY = [1, 2, 3, 4] as const

export type CCADay = (typeof CCA_DAYS)[number]
export type CCASlotNumber = (typeof CCA_SLOTS_PER_DAY)[number]

const CCA_DAY_SHORT_LABELS: Record<CCADay, string> = {
	Monday: "Mon",
	Tuesday: "Tue",
	Wednesday: "Wed",
	Thursday: "Thu",
}

export function ccaTimeSlotID(day: CCADay, slot: CCASlotNumber): string {
	return `${day} CCA ${slot}`
}

export function formatCCATimeSlotLabel(timeSlotID: string): string {
	for (const day of CCA_DAYS) {
		const prefix = `${day} CCA `
		if (timeSlotID.startsWith(prefix)) {
			return `${CCA_DAY_SHORT_LABELS[day]} CCA ${timeSlotID.slice(prefix.length)}`
		}
	}
	return timeSlotID
}

export const FIXED_CCA_TIME_SLOTS = CCA_DAYS.flatMap((day) =>
	CCA_SLOTS_PER_DAY.map((slot) => ccaTimeSlotID(day, slot)),
)
