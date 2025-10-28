import type { Course } from '@/types'

export const sampleCCAs: Course[] = [
  { id: 'CCA001', name: 'Basketball', description: 'Learn basketball skills and teamwork', period: 'MW1', max_students: 30, current_students: 25, membership: 'free', teacher: 'Mr. Smith', location: 'Sports Hall A', category_id: 'Sports' },
  { id: 'CCA002', name: 'Chess Club', description: 'Strategic thinking and problem solving', period: 'MW1', max_students: 20, current_students: 15, membership: 'invite_only', teacher: 'Ms. Johnson', location: 'Room 201', category_id: 'Academic' },
  { id: 'CCA003', name: 'Drama', description: 'Acting and performance arts', period: 'MW1', max_students: 25, current_students: 20, membership: 'invite_only', teacher: 'Mrs. Lee', location: 'Theater', category_id: 'Arts' },
  { id: 'CCA004', name: 'Robotics', description: 'Build and program robots', period: 'MW2', max_students: 15, current_students: 12, membership: 'invite_only', teacher: 'Mr. Chen', location: 'Lab 3', category_id: 'STEM' },
  { id: 'CCA005', name: 'Art Club', description: 'Painting and drawing techniques', period: 'MW2', max_students: 20, current_students: 18, membership: 'free', teacher: 'Ms. Wong', location: 'Art Room', category_id: 'Arts' },
  { id: 'CCA006', name: 'Debate', description: 'Public speaking and argumentation skills', period: 'MW2', max_students: 18, current_students: 16, membership: 'free', teacher: 'Mr. Tan', location: 'Room 305', category_id: 'Academic' },
  { id: 'CCA007', name: 'Soccer', description: 'Football training and matches', period: 'TT1', max_students: 30, current_students: 28, membership: 'free', teacher: 'Coach Brown', location: 'Field 1', category_id: 'Sports' },
  { id: 'CCA008', name: 'Music Band', description: 'Play instruments in ensemble', period: 'TT1', max_students: 25, current_students: 22, membership: 'free', teacher: 'Ms. Garcia', location: 'Music Room', category_id: 'Arts' },
  { id: 'CCA009', name: 'Coding Club', description: 'Learn programming fundamentals', period: 'TT2', max_students: 20, current_students: 19, membership: 'free', teacher: 'Mr. Kumar', location: 'Computer Lab', category_id: 'STEM' },
]
