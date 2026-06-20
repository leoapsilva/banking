// Package cronworker wraps robfig/cron with structured logging so feature
// packages only need to provide a name and a func(context.Context) error.
package cronworker

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"
)

// Worker runs scheduled jobs in-process for the lifetime of the binary.
type Worker struct {
	cron *cron.Cron
}

// New creates a worker. Schedules use standard cron expressions
// (e.g. "@hourly", "0 * * * *").
func New() *Worker {
	return &Worker{cron: cron.New()}
}

// Job is a unit of recurring work. Errors are logged; the scheduler keeps
// running subsequent ticks regardless of a failed run.
type Job func(ctx context.Context) error

// Schedule registers a named job on the given cron expression.
func (w *Worker) Schedule(name, expr string, job Job) error {
	_, err := w.cron.AddFunc(expr, func() {
		ctx := context.Background()
		if err := job(ctx); err != nil {
			slog.Error("cronworker: job failed", "job", name, "error", err)
			return
		}
		slog.Info("cronworker: job finished", "job", name)
	})
	return err
}

// Start begins running scheduled jobs in the background.
func (w *Worker) Start() {
	w.cron.Start()
}

// Stop halts the scheduler, waiting for any running job to finish.
func (w *Worker) Stop() {
	<-w.cron.Stop().Done()
}
