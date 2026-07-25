package httpapi

import (
	"context"

	"git.sr.ht/~runxiyu/cca/internal/courses"
)

// CourseInput is the editable course payload accepted by HTTP handlers.
type CourseInput = courses.Input

var errCourseNeedsPeriod = courses.ErrNeedsPeriod

func normalizeStringSet(values []string) []string {
	return courses.NormalizeStringSet(values)
}

func (app *App) createCourse(ctx context.Context, input CourseInput) error {
	return app.courseService.Create(ctx, input)
}

func (app *App) updateCourse(ctx context.Context, input CourseInput) error {
	return app.courseService.Update(ctx, input)
}
