package engine

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Mode is what a job does.
type Mode string

const (
	// ModeDownload fetches the timeline, saves it to the library and
	// downloads its media. The default.
	ModeDownload Mode = "download"
	// ModeFetch fetches and saves to the library without downloading.
	ModeFetch Mode = "fetch"
	// ModeLibrary downloads what the library already holds, fetching
	// nothing.
	ModeLibrary Mode = "library"
)

// State is where a job is in its life.
type State string

const (
	StateQueued      State = "queued"
	StateFetching    State = "fetching"
	StateDownloading State = "downloading"
	StateDone        State = "done"
	StateFailed      State = "failed"
	StateCancelled   State = "cancelled"
)

// Finished reports whether the job will not change any more.
func (s State) Finished() bool {
	return s == StateDone || s == StateFailed || s == StateCancelled
}

// StatusDownloadedBefore is the item status of what the download history
// already holds.
const StatusDownloadedBefore = "downloaded-before"

// TimelineTypes are the timelines a job can fetch.
var TimelineTypes = []string{"media", "timeline", "tweets", "with_replies", "likes", "bookmarks"}

// Request asks for a job. Only Username is required (except for
// bookmarks); the pointer fields override the saved settings.
type Request struct {
	Username     string `json:"username"`
	TimelineType string `json:"timeline_type,omitempty"`
	Mode         Mode   `json:"mode,omitempty"`
	// Full disables the incremental refetch for this job.
	Full bool `json:"full,omitempty"`
	// Redownload ignores the download history (files on disk are still
	// skipped when the settings say so).
	Redownload bool `json:"redownload,omitempty"`
	// Source names who asked: gui, cli, extension, api.
	Source string `json:"source,omitempty"`

	OutputDir     string `json:"output_dir,omitempty"`
	IncludePhotos *bool  `json:"include_photos,omitempty"`
	IncludeVideos *bool  `json:"include_videos,omitempty"`
	IncludeGifs   *bool  `json:"include_gifs,omitempty"`
	IncludeText   *bool  `json:"include_text,omitempty"`
	Retweets      *bool  `json:"retweets,omitempty"`
	// Only downloads these media URLs of the library's copy, as a profile
	// page's "download this"; it implies ModeLibrary.
	Only []string `json:"only,omitempty"`
}

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

// NormalizeHandle turns "@user", "x.com/user" or a profile URL into a
// bare handle.
func NormalizeHandle(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "/") {
		if !strings.Contains(s, "://") {
			s = "https://" + s
		}
		if u, err := url.Parse(s); err == nil {
			segs := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(segs) > 0 {
				s = segs[0]
			}
		}
	}
	return strings.TrimPrefix(s, "@")
}

// Validate normalizes the request and reports what is wrong with it.
func (r *Request) Validate() error {
	r.Username = NormalizeHandle(r.Username)
	r.TimelineType = strings.ToLower(strings.TrimSpace(r.TimelineType))
	if r.TimelineType == "" {
		r.TimelineType = "media"
	}
	known := false
	for _, t := range TimelineTypes {
		if t == r.TimelineType {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("unknown timeline type %q (one of %s)", r.TimelineType, strings.Join(TimelineTypes, ", "))
	}
	if len(r.Only) > 0 {
		r.Mode = ModeLibrary
	}
	switch r.Mode {
	case "":
		r.Mode = ModeDownload
	case ModeDownload, ModeFetch, ModeLibrary:
	default:
		return fmt.Errorf("unknown mode %q (download, fetch or library)", r.Mode)
	}
	if r.Source == "" {
		r.Source = "api"
	}
	if r.TimelineType == "bookmarks" {
		r.Username = "bookmarks"
		return nil
	}
	if r.Username == "" {
		return errors.New("username is required")
	}
	if !handlePattern.MatchString(r.Username) {
		return fmt.Errorf("invalid username %q", r.Username)
	}
	return nil
}

// Account is what a job learned about the account.
type Account struct {
	Name           string `json:"name"` // the @handle without @
	Nick           string `json:"nick"` // the display name
	ProfileImage   string `json:"profile_image,omitempty"`
	FollowersCount int    `json:"followers_count,omitempty"`
	StatusesCount  int    `json:"statuses_count,omitempty"`
}

// Job is a snapshot of a job, safe to keep and to marshal.
type Job struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	TimelineType string `json:"timeline_type"`
	Mode         Mode   `json:"mode"`
	Source       string `json:"source"`
	State        State  `json:"state"`

	Pages      int  `json:"pages"`      // timeline pages fetched
	Fetched    int  `json:"fetched"`    // entries in the timeline
	New        int  `json:"new"`        // entries not in the library before
	Total      int  `json:"total"`      // items to download
	Current    int  `json:"current"`    // items processed so far
	Downloaded int  `json:"downloaded"` // newly downloaded
	Skipped    int  `json:"skipped"`    // already on disk
	Known      int  `json:"known"`      // in the download history: no request made
	Failed     int  `json:"failed"`     // failed to download
	Percent    int  `json:"percent"`    // of the download
	Incomplete bool `json:"incomplete"` // the fetch stopped before the end

	Account   *Account `json:"account,omitempty"`
	OutputDir string   `json:"output_dir,omitempty"`
	Message   string   `json:"message,omitempty"`
	Error     string   `json:"error,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Summary is a one-line description of the job's progress.
func (j Job) Summary() string {
	switch j.State {
	case StateQueued:
		return "Queued"
	case StateFetching:
		if j.Pages == 0 {
			return "Fetching…"
		}
		return fmt.Sprintf("Fetching — page %d, %d items", j.Pages, j.Fetched)
	case StateDownloading:
		return fmt.Sprintf("Downloading %d/%d", j.Current, j.Total)
	case StateFailed:
		return "Failed: " + j.Error
	}
	if j.Message != "" {
		return j.Message
	}
	return string(j.State)
}

// Label is how the job names its target: @user, or @user's likes.
func (j Job) Label() string {
	switch j.TimelineType {
	case "bookmarks":
		return "Bookmarks"
	case "media", "":
		return "@" + j.Username
	case "with_replies":
		return "@" + j.Username + " (with replies)"
	default:
		return "@" + j.Username + " (" + j.TimelineType + ")"
	}
}
