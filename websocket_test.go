package main

import "testing"

func TestCourseStateUpdatesCoalesceByNewestRevision(t *testing.T) {
	hub := NewWebSocketHub()
	hub.PublishCourseStates([]CourseStateUpdate{
		{CourseID: "B", CurrentStudents: 2, StateRevision: 4},
		{CourseID: "A", CurrentStudents: 1, StateRevision: 3},
		{CourseID: "A", CurrentStudents: 2, StateRevision: 5},
		{CourseID: "A", CurrentStudents: 99, StateRevision: 4},
		{CourseID: "", CurrentStudents: 1, StateRevision: 6},
		{CourseID: "C", CurrentStudents: -1, StateRevision: 6},
	})

	states := hub.takeCourseStates()
	if len(states) != 2 {
		t.Fatalf("coalesced states = %v, want two valid courses", states)
	}
	if states[0] != (CourseStateUpdate{
		CourseID:        "A",
		CurrentStudents: 2,
		StateRevision:   5,
	}) {
		t.Fatalf("course A state = %+v, want newest revision", states[0])
	}
	if states[1] != (CourseStateUpdate{
		CourseID:        "B",
		CurrentStudents: 2,
		StateRevision:   4,
	}) {
		t.Fatalf("course B state = %+v", states[1])
	}
}
