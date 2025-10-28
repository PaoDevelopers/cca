export type LegalSex = 'F' | 'M' | 'X'
export type SelectionType = 'no' | 'invite' | 'force'
export type MembershipType = 'free' | 'invite_only'

export interface Grade {
  grade: string
  enabled: boolean
}

export interface Category {
  id: string
}

export interface Period {
  id: string
}

export interface Admin {
  id: number
  username: string
  session_token: string | null
}

export interface Student {
  id: number
  name: string
  grade: string
  legal_sex: LegalSex
  session_token: string | null
}

export interface Course {
  id: string
  name: string
  description: string
  period: string
  max_students: number
  current_students: number
  membership: MembershipType
  teacher: string
  location: string
  category_id: string
}

export interface Choice {
  student_id: number
  course_id: string
  period: string
  selection_type: SelectionType
}
