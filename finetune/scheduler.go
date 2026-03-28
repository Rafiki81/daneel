package finetune

import (
	"context"
	"log/slog"
	"time"
)

// Scheduler automatically triggers retraining when conditions are met.
type Scheduler struct {
	collector  *Collector
	threshold  int
	interval   time.Duration
	baseConfig Config
	onComplete func(Result)
	onError    func(error)
}

// SchedulerOption configures a Scheduler.
type SchedulerOption func(*Scheduler)

// CollectFrom sets the collector to monitor for new data.
func CollectFrom(c *Collector) SchedulerOption {
	return func(s *Scheduler) { s.collector = c }
}

// RetrainAfter triggers retraining after n new conversations.
func RetrainAfter(n int) SchedulerOption {
	return func(s *Scheduler) { s.threshold = n }
}

// RetrainEvery triggers retraining on a time interval.
func RetrainEvery(d time.Duration) SchedulerOption {
	return func(s *Scheduler) { s.interval = d }
}

// BaseConfig sets the training configuration template.
func BaseConfig(cfg Config) SchedulerOption {
	return func(s *Scheduler) { s.baseConfig = cfg }
}

// OnComplete is called when training succeeds.
func OnComplete(fn func(Result)) SchedulerOption {
	return func(s *Scheduler) { s.onComplete = fn }
}

// OnError is called when training fails.
func OnError(fn func(error)) SchedulerOption {
	return func(s *Scheduler) { s.onError = fn }
}

// NewScheduler creates a training scheduler.
func NewScheduler(opts ...SchedulerOption) *Scheduler {
	s := &Scheduler{
		threshold: 1000,
		interval:  7 * 24 * time.Hour,
		onComplete: func(r Result) {
			slog.Info("training complete", "path", r.OutputPath, "duration", r.Duration)
		},
		onError: func(err error) {
			slog.Error("training failed", "error", err)
		},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Start runs the scheduler loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	lastCount := 0
	if s.collector != nil {
		lastCount = s.collector.Count()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			shouldTrain := false

			if s.collector != nil {
				current := s.collector.Count()
				if current-lastCount >= s.threshold {
					shouldTrain = true
					lastCount = current
				}
			} else {
				shouldTrain = true // time-based only
			}

			if shouldTrain {
				s.runTraining(ctx)
			}
		}
	}
}

func (s *Scheduler) runTraining(ctx context.Context) {
	slog.Info("scheduler: starting training run")

	job, err := Run(ctx, s.baseConfig.DataPath,
		func(c *Config) { *c = s.baseConfig },
	)
	if err != nil {
		s.onError(err)
		return
	}

	result, err := job.Wait()
	if err != nil {
		s.onError(err)
		return
	}

	s.onComplete(result)
}
