package web

// The hub's refresher reads the fresh counts and fans them out; this
// never blocks.
func (app *Server) broadcastCourseCounts(courseIDs []string) {
	app.wsHub.MarkCoursesDirty(courseIDs...)
}
