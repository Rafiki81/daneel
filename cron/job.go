package cron

import (
	"sync"
	"time"

	"github.com/daneel-ai/daneel"
)

// Job represents a scheduled task.
type Job struct {
	ID         int64
	Agent      *daneel.Agent
	Input      string
	expr       string
	schedule   *Schedule
	interval   time.Duration
	mu         sync.Mutex
	LastRun    time.Time
	NextRun    time.Time
	RunCount   int
	ErrCount   int
	LastError  error
	sessionID  string
	timeout    time.Duration
	maxRetries int
	callback   func(*daneel.RunResult, error)
}

// JobOption configures a Job.
type JobOption func(*Job)

// WithSession sets the session ID passed to the agent on every run.
func WithSession(id string) JobOption {
	return func(j *Job) { j.sessionID = id }
}

// WithCallback registers a function called after each run.
func WithCallback(fn func(*daneel.RunResult, error)) JobOption {
	return func(j *Job) { j.callback = fn }
}

// WithTimeout sets a per-run timeout (0 = no timeout).
func WithTimeout(d time.Duration) JobOption {
	return func(j *Job) { j.timeout = d }
}

// WithMaxRetries sets how many times a failed run is retried.
func WithMaxRetries(n int) JobOption {
	return func(j *Job) { j.maxRetries = n }
}

func newJob(id int64, agent *daneel.Agent, input string, opts ...JobOption) *Job {
	j := &Job{ID: id, Agent: agent, Input: input}
	for _, o := range opts {
		o(j)
	}
	return j
}

func (j *Job) computeNext(now time.Time) time.Time {
	if j.schedule != nil {
		return j.schedule.Next(now)
	}
	return now.Add(j.interval)
}

// Expression returns the cron expression or interval string for display.
func (j *Job) Expression() string {
	if j.expr != "" {
		return j.expr
	}
	return j.interval.String()
}
