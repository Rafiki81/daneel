package cron

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/daneel-ai/daneel"
)

// Scheduler runs daneel agents on a schedule.
type Scheduler struct {
	mu     sync.Mutex
	jobs   []*Job
	nextID atomic.Int64
	done   chan struct{}
}

// New creates a new Scheduler. Call Start to begin execution.
func New() *Scheduler {
	return &Scheduler{done: make(chan struct{})}
}

// Schedule adds a job that runs according to a 5-field cron expression.
func (s *Scheduler) Schedule(expr string, agent *daneel.Agent, input string, opts ...JobOption) error {
	sched, err := Parse(expr)
	if err != nil {
		return fmt.Errorf("cron: parse expression: %w", err)
	}
	id := s.nextID.Add(1)
	j := newJob(id, agent, input, opts...)
	j.expr = expr
	j.schedule = sched
	j.NextRun = sched.Next(time.Now())
	s.mu.Lock()
	s.jobs = append(s.jobs, j)
	s.mu.Unlock()
	return nil
}

// Every adds a job that runs on a fixed interval.
func (s *Scheduler) Every(interval time.Duration, agent *daneel.Agent, input string, opts ...JobOption) {
	id := s.nextID.Add(1)
	j := newJob(id, agent, input, opts...)
	j.interval = interval
	j.NextRun = time.Now().Add(interval)
	s.mu.Lock()
	s.jobs = append(s.jobs, j)
	s.mu.Unlock()
}

// Start begins the scheduling loop. It blocks until ctx is cancelled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			due := make([]*Job, 0)
			for _, j := range s.jobs {
				j.mu.Lock()
				if !j.NextRun.IsZero() && !now.Before(j.NextRun) {
					due = append(due, j)
				}
				j.mu.Unlock()
			}
			s.mu.Unlock()
			for _, j := range due {
				go s.runJob(ctx, j)
			}
		}
	}
}

// Stop signals the scheduler to stop after the current tick.
func (s *Scheduler) Stop() {
	close(s.done)
}

// Jobs returns a snapshot of registered jobs.
func (s *Scheduler) Jobs() []*Job {
	s.mu.Lock()
	out := make([]*Job, len(s.jobs))
	copy(out, s.jobs)
	s.mu.Unlock()
	return out
}

func (s *Scheduler) runJob(ctx context.Context, j *Job) {
	j.mu.Lock()
	j.LastRun = time.Now()
	j.NextRun = j.computeNext(j.LastRun)
	j.mu.Unlock()

	runCtx := ctx
	var cancel context.CancelFunc
	if j.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, j.timeout)
		defer cancel()
	}

	opts := []daneel.RunOption{}
	if j.sessionID != "" {
		opts = append(opts, daneel.WithSessionID(j.sessionID))
	}

	attempts := 1 + j.maxRetries
	var result *daneel.RunResult
	var err error
	for i := 0; i < attempts; i++ {
		result, err = daneel.Run(runCtx, j.Agent, j.Input, opts...)
		if err == nil {
			break
		}
	}

	j.mu.Lock()
	j.RunCount++
	if err != nil {
		j.ErrCount++
		j.LastError = err
	} else {
		j.LastError = nil
	}
	j.mu.Unlock()

	if j.callback != nil {
		j.callback(result, err)
	}
}
