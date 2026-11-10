// Package engine runs XDL's jobs: fetching an account's timeline from X,
// saving it to the library and downloading its media. The GUI, the
// CLI and the HTTP API (and so the browser extension) all submit jobs to the
// same queue, which runs one job at a time.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/settings"
)

// maxHistory is how many finished jobs the engine remembers.
const maxHistory = 200

// EventType names what an Event reports.
type EventType string

const (
	// EventJob carries a job's new snapshot: it was queued, progressed,
	// changed state or was removed (Removed set).
	EventJob EventType = "job"
	// EventItem reports one media item's download status.
	EventItem EventType = "item"
	// EventLibrary reports that the account library changed.
	EventLibrary EventType = "library"
	// EventSubscriptions reports that a subscription changed.
	EventSubscriptions EventType = "subscriptions"
)

// Event is something that happened in the engine.
type Event struct {
	Type    EventType  `json:"type"`
	Job     *Job       `json:"job,omitempty"`
	Item    *ItemEvent `json:"item,omitempty"`
	Removed bool       `json:"removed,omitempty"`
}

// ItemEvent is the download status of one item of a job.
type ItemEvent struct {
	JobID  string `json:"job_id"`
	Index  int    `json:"index"`
	Status string `json:"status"` // success, skipped, failed
	Path   string `json:"path,omitempty"`
}

// Item is one media item of a job, with its download status.
type Item struct {
	backend.TimelineEntry
	Status string `json:"status,omitempty"` // "", success, skipped, failed
	Path   string `json:"path,omitempty"`
}

type job struct {
	Job
	ctx     context.Context
	cancel  context.CancelFunc
	req     Request
	entries []backend.TimelineEntry
	status  []string
	paths   []string
}

// Engine is the job queue.
type Engine struct {
	settings *settings.Store

	mu      sync.Mutex
	jobs    []*job // oldest first
	wake    chan struct{}
	subs    map[int]chan Event
	nextSub int
}

// New returns an engine that reads its preferences from store. Call Run to
// start working through the queue.
func New(store *settings.Store) *Engine {
	return &Engine{
		settings: store,
		wake:     make(chan struct{}, 1),
		subs:     make(map[int]chan Event),
	}
}

// Settings is the store the engine reads its preferences from.
func (e *Engine) Settings() *settings.Store { return e.settings }

// Run works through the queue until ctx is done. It cancels the running
// job on the way out.
func (e *Engine) Run(ctx context.Context) {
	for {
		j := e.nextQueued()
		if j == nil {
			select {
			case <-ctx.Done():
				return
			case <-e.wake:
				continue
			}
		}
		stop := context.AfterFunc(ctx, j.cancel)
		e.run(j)
		stop()
		if ctx.Err() != nil {
			e.cancelAll()
			return
		}
	}
}

func (e *Engine) nextQueued() *job {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.jobs {
		if j.State == StateQueued {
			return j
		}
	}
	return nil
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ErrDuplicate is returned with the existing job by Submit when the same
// account and timeline is already queued or running.
var ErrDuplicate = errors.New("already queued")

// Submit queues a job. A request for an account and timeline already
// queued or running returns that job and ErrDuplicate.
func (e *Engine) Submit(req Request) (Job, error) {
	if err := req.Validate(); err != nil {
		return Job{}, err
	}
	e.mu.Lock()
	for _, j := range e.jobs {
		if !j.State.Finished() && strings.EqualFold(j.Username, req.Username) &&
			j.TimelineType == req.TimelineType && j.Mode == req.Mode {
			snap := j.Job
			e.mu.Unlock()
			return snap, ErrDuplicate
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{
		Job: Job{
			ID:           newID(),
			Username:     req.Username,
			TimelineType: req.TimelineType,
			Mode:         req.Mode,
			Source:       req.Source,
			State:        StateQueued,
			CreatedAt:    time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
		req:    req,
	}
	e.jobs = append(e.jobs, j)
	e.trimLocked()
	snap := j.Job
	e.mu.Unlock()

	log.Printf("job %s queued: %s %s (from %s)", snap.ID, snap.Mode, snap.Label(), snap.Source)
	e.publish(Event{Type: EventJob, Job: &snap})
	select {
	case e.wake <- struct{}{}:
	default:
	}
	return snap, nil
}

// trimLocked forgets the oldest finished jobs beyond maxHistory.
func (e *Engine) trimLocked() {
	finished := 0
	for _, j := range e.jobs {
		if j.State.Finished() {
			finished++
		}
	}
	for i := 0; finished > maxHistory && i < len(e.jobs); {
		if e.jobs[i].State.Finished() {
			e.jobs = slices.Delete(e.jobs, i, i+1)
			finished--
			continue
		}
		i++
	}
}

// Jobs returns every job the engine knows, newest first.
func (e *Engine) Jobs() []Job {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Job, len(e.jobs))
	for i, j := range e.jobs {
		out[len(e.jobs)-1-i] = j.Job
	}
	return out
}

// Get returns one job.
func (e *Engine) Get(id string) (Job, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if j := e.findLocked(id); j != nil {
		return j.Job, true
	}
	return Job{}, false
}

func (e *Engine) findLocked(id string) *job {
	for _, j := range e.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// Active returns the running job, if any.
func (e *Engine) Active() (Job, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.jobs {
		if j.State == StateFetching || j.State == StateDownloading {
			return j.Job, true
		}
	}
	return Job{}, false
}

// Counts returns how many jobs are queued and running.
func (e *Engine) Counts() (queued, running int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, j := range e.jobs {
		switch j.State {
		case StateQueued:
			queued++
		case StateFetching, StateDownloading:
			running++
		}
	}
	return
}

// Items returns the media items of a job, with their download status, once
// its timeline is known.
func (e *Engine) Items(id string) ([]Item, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j := e.findLocked(id)
	if j == nil {
		return nil, false
	}
	out := make([]Item, len(j.entries))
	for i, entry := range j.entries {
		out[i] = Item{TimelineEntry: entry, Status: j.status[i], Path: j.paths[i]}
	}
	return out, true
}

// Cancel stops a queued or running job.
func (e *Engine) Cancel(id string) bool {
	e.mu.Lock()
	j := e.findLocked(id)
	if j == nil || j.State.Finished() {
		e.mu.Unlock()
		return false
	}
	queued := j.State == StateQueued
	j.cancel()
	var snap Job
	if queued {
		now := time.Now()
		j.State, j.FinishedAt, j.Message = StateCancelled, &now, "Cancelled"
		snap = j.Job
	}
	e.mu.Unlock()
	if queued {
		e.publish(Event{Type: EventJob, Job: &snap})
	}
	return true
}

func (e *Engine) cancelAll() {
	e.mu.Lock()
	ids := make([]string, 0)
	for _, j := range e.jobs {
		if !j.State.Finished() {
			ids = append(ids, j.ID)
		}
	}
	e.mu.Unlock()
	for _, id := range ids {
		e.Cancel(id)
	}
}

// Remove forgets a finished job.
func (e *Engine) Remove(id string) bool {
	e.mu.Lock()
	j := e.findLocked(id)
	if j == nil || !j.State.Finished() {
		e.mu.Unlock()
		return false
	}
	e.jobs = slices.DeleteFunc(e.jobs, func(x *job) bool { return x == j })
	snap := j.Job
	e.mu.Unlock()
	e.publish(Event{Type: EventJob, Job: &snap, Removed: true})
	return true
}

// ClearFinished forgets every finished job and returns how many.
func (e *Engine) ClearFinished() int {
	e.mu.Lock()
	var removed []Job
	e.jobs = slices.DeleteFunc(e.jobs, func(j *job) bool {
		if j.State.Finished() {
			removed = append(removed, j.Job)
			return true
		}
		return false
	})
	e.mu.Unlock()
	for i := range removed {
		e.publish(Event{Type: EventJob, Job: &removed[i], Removed: true})
	}
	return len(removed)
}

// Subscribe returns a channel of events and a function that ends the
// subscription. A subscriber that falls behind misses events rather than
// slowing the engine down.
func (e *Engine) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 512)
	e.mu.Lock()
	id := e.nextSub
	e.nextSub++
	e.subs[id] = ch
	e.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			e.mu.Lock()
			delete(e.subs, id)
			e.mu.Unlock()
			close(ch)
		})
	}
}

func (e *Engine) publish(ev Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ch := range e.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// update changes a job under the lock and publishes its snapshot.
func (e *Engine) update(j *job, fn func(*Job)) {
	e.mu.Lock()
	fn(&j.Job)
	if j.Total > 0 {
		j.Percent = j.Current * 100 / j.Total
	}
	snap := j.Job
	e.mu.Unlock()
	e.publish(Event{Type: EventJob, Job: &snap})
}

// NotifyLibraryChanged tells subscribers that the account library changed
// outside a job (the GUI deleted an account, say).
func (e *Engine) NotifyLibraryChanged() {
	e.publish(Event{Type: EventLibrary})
}

// NotifySubscriptionsChanged tells subscribers that a subscription was
// added, changed, removed or checked.
func (e *Engine) NotifySubscriptionsChanged() {
	e.publish(Event{Type: EventSubscriptions})
}
