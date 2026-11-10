package backend

import (
	"database/sql"
	"errors"
	"time"
)

// Subscriptions are accounts XDL checks on a schedule, downloading what
// they posted since.

const subscriptionSchema = `
CREATE TABLE IF NOT EXISTS subscriptions (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	username       TEXT NOT NULL,
	timeline_type  TEXT NOT NULL DEFAULT 'media',
	interval_mins  INTEGER NOT NULL DEFAULT 1440,
	enabled        INTEGER NOT NULL DEFAULT 1,
	created_at     DATETIME NOT NULL,
	last_run       DATETIME,
	next_run       DATETIME NOT NULL,
	last_job       TEXT NOT NULL DEFAULT '',
	last_status    TEXT NOT NULL DEFAULT '',
	last_message   TEXT NOT NULL DEFAULT '',
	UNIQUE(username, timeline_type)
)`

// Subscription is one scheduled account.
type Subscription struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	TimelineType string     `json:"timeline_type"`
	IntervalMins int        `json:"interval_minutes"`
	Enabled      bool       `json:"enabled"`
	CreatedAt    time.Time  `json:"created_at"`
	LastRun      *time.Time `json:"last_run,omitempty"`
	NextRun      time.Time  `json:"next_run"`
	LastJob      string     `json:"last_job,omitempty"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastMessage  string     `json:"last_message,omitempty"`
}

// ErrNoSubscription is returned for an unknown subscription.
var ErrNoSubscription = errors.New("no such subscription")

const subscriptionColumns = `id, username, timeline_type, interval_mins, enabled, created_at, last_run, next_run, last_job, last_status, last_message`

func scanSubscription(row interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	var enabled int
	var lastRun sql.NullTime
	err := row.Scan(&s.ID, &s.Username, &s.TimelineType, &s.IntervalMins, &enabled,
		&s.CreatedAt, &lastRun, &s.NextRun, &s.LastJob, &s.LastStatus, &s.LastMessage)
	if err != nil {
		return s, err
	}
	s.Enabled = enabled != 0
	if lastRun.Valid {
		t := lastRun.Time
		s.LastRun = &t
	}
	return s, nil
}

// ListSubscriptions returns every subscription, by account.
func ListSubscriptions() ([]Subscription, error) {
	if err := ensureDB(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT ` + subscriptionColumns + ` FROM subscriptions ORDER BY lower(username), timeline_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSubscription returns one subscription.
func GetSubscription(id int64) (Subscription, error) {
	if err := ensureDB(); err != nil {
		return Subscription{}, err
	}
	s, err := scanSubscription(db.QueryRow(`SELECT `+subscriptionColumns+` FROM subscriptions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNoSubscription
	}
	return s, err
}

// UpsertSubscription subscribes to an account's timeline, or changes the
// interval of an existing subscription and enables it. The first check is
// due at once.
func UpsertSubscription(username, timelineType string, intervalMins int) (Subscription, error) {
	if err := ensureDB(); err != nil {
		return Subscription{}, err
	}
	now := time.Now()
	_, err := db.Exec(`INSERT INTO subscriptions (username, timeline_type, interval_mins, enabled, created_at, next_run)
		VALUES (?, ?, ?, 1, ?, ?)
		ON CONFLICT(username, timeline_type) DO UPDATE SET interval_mins = excluded.interval_mins, enabled = 1`,
		username, timelineType, intervalMins, now, now)
	if err != nil {
		return Subscription{}, err
	}
	return scanSubscription(db.QueryRow(`SELECT `+subscriptionColumns+` FROM subscriptions
		WHERE lower(username) = lower(?) AND timeline_type = ?`, username, timelineType))
}

// UpdateSubscription changes a subscription's interval and whether it runs.
// A zero interval keeps the current one.
func UpdateSubscription(id int64, intervalMins int, enabled bool) (Subscription, error) {
	s, err := GetSubscription(id)
	if err != nil {
		return s, err
	}
	if intervalMins <= 0 {
		intervalMins = s.IntervalMins
	}
	next := s.NextRun
	if s.LastRun != nil {
		next = s.LastRun.Add(time.Duration(intervalMins) * time.Minute)
	}
	if _, err := db.Exec(`UPDATE subscriptions SET interval_mins = ?, enabled = ?, next_run = ? WHERE id = ?`,
		intervalMins, boolInt(enabled), next, id); err != nil {
		return s, err
	}
	return GetSubscription(id)
}

// MarkSubscriptionRun records that a check started as job jobID and when the
// next one is due.
func MarkSubscriptionRun(id int64, jobID string, at time.Time) error {
	sub, err := GetSubscription(id)
	if err != nil {
		return err
	}
	next := at.Add(time.Duration(sub.IntervalMins) * time.Minute)
	_, err = db.Exec(`UPDATE subscriptions SET last_run = ?, next_run = ?, last_job = ?, last_status = 'running',
		last_message = '' WHERE id = ?`, at, next, jobID, id)
	return err
}

// FinishSubscriptionRun records how the check of job jobID ended. A
// non-zero retry brings the next check forward to then, when it is sooner.
func FinishSubscriptionRun(jobID, status, message string, retry time.Time) error {
	if err := ensureDB(); err != nil || jobID == "" {
		return err
	}
	_, err := db.Exec(`UPDATE subscriptions SET last_status = ?, last_message = ? WHERE last_job = ?`, status, message, jobID)
	if err != nil || retry.IsZero() {
		return err
	}
	rows, err := db.Query(`SELECT id, next_run FROM subscriptions WHERE last_job = ?`, jobID)
	if err != nil {
		return err
	}
	var sooner []int64
	for rows.Next() {
		var id int64
		var next time.Time
		if rows.Scan(&id, &next) == nil && next.After(retry) {
			sooner = append(sooner, id)
		}
	}
	rows.Close()
	for _, id := range sooner {
		if _, err := db.Exec(`UPDATE subscriptions SET next_run = ? WHERE id = ?`, retry, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSubscription unsubscribes.
func DeleteSubscription(id int64) error {
	if err := ensureDB(); err != nil {
		return err
	}
	res, err := db.Exec(`DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoSubscription
	}
	return nil
}

// DueSubscriptions returns the enabled subscriptions due at now.
func DueSubscriptions(now time.Time) ([]Subscription, error) {
	all, err := ListSubscriptions()
	if err != nil {
		return nil, err
	}
	var due []Subscription
	for _, s := range all {
		if s.Enabled && !s.NextRun.After(now) {
			due = append(due, s)
		}
	}
	return due, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
