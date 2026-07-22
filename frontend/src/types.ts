export type LegalSex = "F" | "M" | "X"
export type SelectionType = "normal" | "invite" | "force"
export type MembershipType = "free" | "invite_only"

export interface Student {
	id: number
	name: string
	grade: string
	legal_sex: LegalSex
}

export interface AdminSession {
	id: number
	username: string
}

export interface StudentSession {
	role: "student"
	student: Student
}

export interface GradeRequirementGroup {
	id: number
	min_count: number
	category_ids: readonly string[]
}

export interface StudentRequirementProgress extends GradeRequirementGroup {
	current_count: number
}

export interface Grade {
	grade: string
	enabled: boolean
	max_own_choices: number
	req_groups: readonly GradeRequirementGroup[]
}

export interface CourseBlockReason {
	code:
		| "no_periods"
		| "course_full"
		| "invite_only"
		| "legal_sex_restricted"
		| "grade_restricted"
		| "selections_closed"
		| "choice_limit"
		| "schedule_conflict"
	message: string
	period_ids?: readonly string[]
	conflicting_course_id?: string
}

export interface Course {
	id: string
	name: string
	description: string
	period_ids: readonly string[]
	max_students: number
	current_students: number
	membership: MembershipType
	teacher: string
	location: string
	category_id: string
	allowed_legal_sexes: readonly LegalSex[]
	allowed_grades: readonly string[]
	selected: boolean
	selected_period_id?: string
	available_period_ids: readonly string[]
	selection_type?: SelectionType
	available: boolean
	block_reasons: readonly CourseBlockReason[]
	removable: boolean
	removal_block_reason?: string
}

export interface Selection {
	student_id?: number
	student_name?: string
	student_grade?: string
	course_id: string
	course_name?: string
	period_id: string
	selection_type: SelectionType
}

export interface GradeSelectionSchedule {
	batch_id: number
	grade_ids: readonly string[]
	opens_at: string
	closes_at?: string
	opened: boolean
}

export interface AdminBootstrap {
	admin: AdminSession
	categories: readonly string[]
	periods: readonly string[]
	grades: readonly Grade[]
	grade_schedules: readonly GradeSelectionSchedule[]
	courses: readonly Course[]
	students: readonly Student[]
	selections: readonly Selection[]
}

export interface StudentBootstrap {
	session: StudentSession
	courses: readonly Course[]
	requirements: readonly StudentRequirementProgress[]
}

export interface CoursePayload {
	id: string
	name: string
	description: string
	period_ids: string[]
	max_students: number
	membership: MembershipType
	teacher: string
	location: string
	category_id: string
	allowed_legal_sexes: LegalSex[]
	allowed_grades: string[]
}

export interface APIErrorEnvelope {
	error: {
		code: string
		message: string
	}
}
