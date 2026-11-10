// Package schedule runs subscriptions: accounts XDL checks on an interval,
// downloading what they posted since. Each check is an ordinary job in the
// engine's queue, incremental (it stops where the library's copy begins) and
// skipping what the download history holds, so a check of an account with
// nothing new costs one timeline page and no downloads.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
)

// MinInterval is the shortest interval a subscription may have.
const MinInterval = 15 * time.Minute

// DefaultInterval is a day.
const DefaultInterval = 24 * time.Hour

// RetryAfter is when a failed check runs again, if its interval is longer.
const RetryAfter = time.Hour

// Scheduler starts the checks of due subscriptions.
type Scheduler struct {
	eng  *engine.Engine
	wake chan struct{}
}

// New returns a scheduler for eng. Only the process serving the API runs
// one, so two processes never check the same account.
func New(eng *engine.Engine) *Scheduler {
	return &Scheduler{eng: eng, wake: make(chan struct{}, 1)}
}

// Run checks for due subscriptions every minute, and records how their
// jobs end, until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	events, unsubscribe := s.eng.Subscribe()
	defer unsubscribe()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	s.runDue()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.runDue()
		case <-s.wake:
			s.runDue()
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Type == engine.EventJob && ev.Job != nil && ev.Job.Source == "schedule" && ev.Job.State.Finished() {
				msg := ev.Job.Message
				var retry time.Time
				if ev.Job.State == engine.StateFailed {
					// A network hiccup or a missing tool: try again soon.
					msg, retry = ev.Job.Error, time.Now().Add(RetryAfter)
				}
				if err := backend.FinishSubscriptionRun(ev.Job.ID, string(ev.Job.State), msg, retry); err != nil {
					log.Printf("schedule: %v", err)
				}
				s.eng.NotifySubscriptionsChanged()
			}
		}
	}
}

func (s *Scheduler) runDue() {
	due, err := backend.DueSubscriptions(time.Now())
	if err != nil {
		log.Printf("schedule: %v", err)
		return
	}
	for _, sub := range due {
		if _, err := s.start(sub); err != nil {
			log.Printf("schedule: @%s: %v", sub.Username, err)
		}
	}
}

func (s *Scheduler) start(sub backend.Subscription) (engine.Job, error) {
	j, err := s.eng.Submit(engine.Request{
		Username:     sub.Username,
		TimelineType: sub.TimelineType,
		Mode:         engine.ModeDownload,
		Source:       "schedule",
	})
	if err != nil && !errors.Is(err, engine.ErrDuplicate) {
		return j, err
	}
	log.Printf("schedule: checking %s (every %s)", j.Label(), Describe(sub.IntervalMins))
	if err := backend.MarkSubscriptionRun(sub.ID, j.ID, time.Now()); err != nil {
		return j, err
	}
	s.eng.NotifySubscriptionsChanged()
	return j, nil
}

// Subscribe adds a subscription (or changes its interval); its first check
// starts at once.
func (s *Scheduler) Subscribe(username, timelineType string, interval time.Duration) (backend.Subscription, error) {
	req := engine.Request{Username: username, TimelineType: timelineType}
	if err := req.Validate(); err != nil {
		return backend.Subscription{}, err
	}
	if interval == 0 {
		interval = DefaultInterval
	}
	if interval < MinInterval {
		return backend.Subscription{}, fmt.Errorf("the interval must be at least %s", MinInterval)
	}
	sub, err := backend.UpsertSubscription(req.Username, req.TimelineType, int(interval/time.Minute))
	if err != nil {
		return sub, err
	}
	s.eng.NotifySubscriptionsChanged()
	s.poke()
	return sub, nil
}

// Update changes a subscription's interval (0 keeps it) and whether it runs.
func (s *Scheduler) Update(id int64, interval time.Duration, enabled bool) (backend.Subscription, error) {
	if interval != 0 && interval < MinInterval {
		return backend.Subscription{}, fmt.Errorf("the interval must be at least %s", MinInterval)
	}
	sub, err := backend.UpdateSubscription(id, int(interval/time.Minute), enabled)
	if err == nil {
		s.eng.NotifySubscriptionsChanged()
		s.poke()
	}
	return sub, err
}

// Unsubscribe removes a subscription; downloads and the library stay.
func (s *Scheduler) Unsubscribe(id int64) error {
	err := backend.DeleteSubscription(id)
	if err == nil {
		s.eng.NotifySubscriptionsChanged()
	}
	return err
}

// RunNow checks a subscription at once, whenever it is due.
func (s *Scheduler) RunNow(id int64) (engine.Job, error) {
	sub, err := backend.GetSubscription(id)
	if err != nil {
		return engine.Job{}, err
	}
	return s.start(sub)
}

func (s *Scheduler) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Describe writes an interval in minutes as people say it.
func Describe(mins int) string {
	d := time.Duration(mins) * time.Minute
	switch {
	case d%(7*24*time.Hour) == 0:
		return plural(int(d/(7*24*time.Hour)), "week")
	case d%(24*time.Hour) == 0:
		return plural(int(d/(24*time.Hour)), "day")
	case d%time.Hour == 0:
		return plural(int(d/time.Hour), "hour")
	}
	return plural(mins, "minute")
}

func plural(n int, unit string) string {
	if n == 1 {
		return unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// ParseInterval reads "24h", "30m", "1d" or "1w".
func ParseInterval(s string) (time.Duration, error) {
	if s == "" {
		return DefaultInterval, nil
	}
	n := len(s)
	var mult time.Duration
	switch s[n-1] {
	case 'd':
		mult = 24 * time.Hour
	case 'w':
		mult = 7 * 24 * time.Hour
	}
	if mult != 0 {
		var v int
		if _, err := fmt.Sscanf(s[:n-1], "%d", &v); err != nil || v <= 0 {
			return 0, fmt.Errorf("invalid interval %q", s)
		}
		return time.Duration(v) * mult, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid interval %q (as 6h, 1d or 1w)", s)
	}
	return d, nil
}
