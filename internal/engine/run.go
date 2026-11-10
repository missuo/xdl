package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/settings"
)

const (
	batchSize       = 100
	maxPages        = 500
	maxEmptyPages   = 3
	progressEvery   = 150 * time.Millisecond
	authTokenSlot   = "public"
	errNoAuthTokenS = "no X auth token saved — add one in Settings (or run `xdl token set`)"
)

// ErrNoAuthToken is the failure of a job that needs X and has no token.
var ErrNoAuthToken = errors.New(errNoAuthTokenS)

// savedTimeline is the JSON the library stores per account, which the
// Library page and exports read too.
type savedTimeline struct {
	AccountInfo backend.AccountInfo     `json:"account_info"`
	TotalURLs   int                     `json:"total_urls"`
	Timeline    []backend.TimelineEntry `json:"timeline"`
	Metadata    savedTimelineMetadata   `json:"metadata"`
	Cursor      string                  `json:"cursor"`
	Completed   bool                    `json:"completed"`
}

type savedTimelineMetadata struct {
	HasMore bool   `json:"has_more"`
	Cursor  string `json:"cursor"`
}

func (e *Engine) run(j *job) {
	now := time.Now()
	e.update(j, func(s *Job) {
		s.StartedAt = &now
		s.State = StateFetching
		if j.req.Mode == ModeLibrary {
			s.State = StateDownloading
		}
	})
	log.Printf("job %s started: %s %s", j.ID, j.Mode, j.Label())

	err := e.pipeline(j)

	finished := time.Now()
	e.update(j, func(s *Job) {
		s.FinishedAt = &finished
		switch {
		case err == nil:
			s.State = StateDone
		case errors.Is(err, context.Canceled):
			s.State = StateCancelled
			if s.Message == "" {
				s.Message = "Cancelled"
			}
		default:
			s.State = StateFailed
			s.Error = err.Error()
		}
	})
	snap, _ := e.Get(j.ID)
	if err != nil {
		log.Printf("job %s %s: %v", j.ID, snap.State, err)
	} else {
		log.Printf("job %s done: %s", j.ID, snap.Message)
	}
}

func (e *Engine) pipeline(j *job) error {
	cfg := e.settings.Get()
	r := j.req
	override := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	override(&cfg.IncludePhotos, r.IncludePhotos)
	override(&cfg.IncludeVideos, r.IncludeVideos)
	override(&cfg.IncludeGifs, r.IncludeGifs)
	override(&cfg.IncludeText, r.IncludeText)
	override(&cfg.Retweets, r.Retweets)
	outputDir := r.OutputDir
	if outputDir == "" {
		outputDir = cfg.DownloadDir()
	}
	e.update(j, func(s *Job) { s.OutputDir = outputDir })

	var saved *savedTimeline
	if r.Mode == ModeLibrary {
		acc, err := backend.GetAccountByUsernameAndMediaType(r.Username, r.TimelineType)
		if err != nil {
			return fmt.Errorf("%s is not in the library: %w", j.Label(), err)
		}
		saved, err = parseSaved(acc.ResponseJSON)
		if err != nil {
			return err
		}
		if len(r.Only) > 0 {
			only := make(map[string]bool, len(r.Only))
			for _, u := range r.Only {
				only[u] = true
			}
			kept := saved.Timeline[:0:0]
			for _, entry := range saved.Timeline {
				if only[entry.URL] {
					kept = append(kept, entry)
				}
			}
			if len(kept) == 0 {
				return errors.New("none of those items is in the library's copy")
			}
			saved.Timeline = kept
		}
		e.setTimeline(j, saved, 0, !saved.Completed)
	} else {
		var err error
		saved, err = e.fetch(j, cfg)
		if err != nil {
			return err
		}
	}

	if r.Mode == ModeFetch {
		e.update(j, func(s *Job) {
			s.Message = fmt.Sprintf("Saved %d items to the library (%d new)", s.Fetched, s.New)
			if s.Incomplete {
				s.Message += " — incomplete"
			}
		})
		return nil
	}
	return e.download(j, cfg, saved, outputDir)
}

// fetch pages through the timeline and saves it to the library.
func (e *Engine) fetch(j *job, cfg settings.Settings) (*savedTimeline, error) {
	token, err := backend.GetStoredAuthToken(authTokenSlot)
	if err != nil {
		return nil, fmt.Errorf("reading the auth token: %w", err)
	}
	if token == "" {
		return nil, ErrNoAuthToken
	}
	r := j.req

	// An incremental refetch stops once pages bring nothing new, and keeps
	// what the library had beyond them.
	var previous *savedTimeline
	known := map[string]bool{}
	if acc, err := backend.GetAccountByUsernameAndMediaType(r.Username, r.TimelineType); err == nil {
		if p, err := parseSaved(acc.ResponseJSON); err == nil {
			previous = p
			for _, entry := range p.Timeline {
				known[entryKey(entry)] = true
			}
		}
	}
	incremental := cfg.Incremental && !r.Full && previous != nil && previous.Completed && len(known) > 0

	var (
		timeline   []backend.TimelineEntry
		account    backend.AccountInfo
		cursor     string
		completed  bool
		reachedOld bool
		empty      int
		newCount   int
		fetchErr   error
	)
	seen := map[string]bool{}
	for page := 1; page <= maxPages; page++ {
		resp, err := e.extractPage(j.ctx, backend.TimelineRequest{
			Username:      r.Username,
			AuthToken:     token,
			TimelineType:  r.TimelineType,
			BatchSize:     batchSize,
			Page:          page,
			MediaType:     r.TimelineType,
			Retweets:      cfg.Retweets,
			Cursor:        cursor,
			IncludePhotos: cfg.IncludePhotos,
			IncludeVideos: cfg.IncludeVideos,
			IncludeGifs:   cfg.IncludeGifs,
			IncludeText:   cfg.IncludeText,
			VideoQuality:  cfg.VideoQuality,
			ImageSize:     cfg.ImageSize,
			Proxy:         cfg.Proxy,
		}, time.Duration(cfg.FetchTimeout)*time.Second)
		if err != nil {
			if errors.Is(err, context.Canceled) && len(timeline) == 0 {
				return nil, err
			}
			fetchErr = err
			break
		}
		account = resp.AccountInfo
		pageNew, pageKnown := 0, 0
		for _, entry := range resp.Timeline {
			key := entryKey(entry)
			if seen[key] {
				continue
			}
			seen[key] = true
			timeline = append(timeline, entry)
			if known[key] {
				pageKnown++
			} else {
				pageNew++
			}
		}
		newCount += pageNew
		e.update(j, func(s *Job) {
			s.Pages, s.Fetched, s.New = page, len(timeline), newCount
			s.Account = accountOf(account, r.Username)
		})

		next := resp.Metadata.Cursor
		if next == "" {
			next = resp.Cursor
		}
		if !resp.Metadata.HasMore {
			completed, cursor = true, next
			break
		}
		if incremental && len(resp.Timeline) > 0 && (pageNew == 0 || pageKnown*2 > len(resp.Timeline)) {
			reachedOld, completed, cursor = true, true, next
			break
		}
		if len(resp.Timeline) == 0 {
			empty++
		} else {
			empty = 0
		}
		if next == "" || next == cursor || empty >= maxEmptyPages {
			cursor = next
			break
		}
		cursor = next
	}
	if len(timeline) == 0 {
		if fetchErr != nil {
			return nil, fetchErr
		}
		return nil, errors.New("no media found")
	}

	if reachedOld {
		// What the library had and this fetch didn't reach.
		for _, entry := range previous.Timeline {
			if key := entryKey(entry); !seen[key] {
				seen[key] = true
				timeline = append(timeline, entry)
			}
		}
	}

	result := &savedTimeline{
		AccountInfo: account,
		TotalURLs:   len(timeline),
		Timeline:    timeline,
		Metadata:    savedTimelineMetadata{HasMore: !completed, Cursor: cursor},
		Cursor:      cursor,
		Completed:   completed,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if err := backend.SaveAccountWithStatus(r.Username, account.Nick, account.ProfileImage,
		len(timeline), string(data), r.TimelineType, cursor, completed); err != nil {
		log.Printf("job %s: saving to the library: %v", j.ID, err)
	} else {
		e.publish(Event{Type: EventLibrary})
	}
	e.setTimeline(j, result, newCount, !completed)
	if fetchErr != nil {
		if errors.Is(fetchErr, context.Canceled) {
			return nil, fetchErr
		}
		log.Printf("job %s: fetch stopped early: %v", j.ID, fetchErr)
		e.update(j, func(s *Job) { s.Message = "Fetch stopped early: " + fetchErr.Error() })
	}
	return result, nil
}

// extractPage fetches one page, giving up after timeout.
func (e *Engine) extractPage(ctx context.Context, req backend.TimelineRequest, timeout time.Duration) (*backend.TwitterResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := backend.ExtractTimeline(ctx, req)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("page %d timed out after %s", req.Page, timeout)
	}
	return resp, err
}

func (e *Engine) setTimeline(j *job, t *savedTimeline, newCount int, incomplete bool) {
	e.mu.Lock()
	j.entries = t.Timeline
	j.status = make([]string, len(t.Timeline))
	j.paths = make([]string, len(t.Timeline))
	e.mu.Unlock()
	e.update(j, func(s *Job) {
		s.Fetched = len(t.Timeline)
		s.New = newCount
		s.Incomplete = incomplete
		s.Account = accountOf(t.AccountInfo, j.req.Username)
	})
}

func (e *Engine) download(j *job, cfg settings.Settings, t *savedTimeline, outputDir string) error {
	r := j.req
	items := make([]backend.MediaItem, len(t.Timeline))
	for i, entry := range t.Timeline {
		username := r.Username
		if (r.TimelineType == "bookmarks" || r.TimelineType == "likes") && entry.AuthorUsername != "" {
			username = entry.AuthorUsername
		}
		original := entry.OriginalFilename
		if original == "" {
			original = backend.ExtractOriginalFilename(entry.URL)
		}
		items[i] = backend.MediaItem{
			URL:              entry.URL,
			Date:             entry.Date,
			TweetID:          int64(entry.TweetID),
			Type:             entry.Type,
			Username:         username,
			AccountName:      t.AccountInfo.Nick,
			Content:          entry.Content,
			OriginalFilename: original,
		}
	}
	// What the download history holds was downloaded before, whatever the
	// file is called now or wherever it went: skip it without a request.
	// So is an item a job lists twice.
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = backend.MediaKey(it.URL, it.TweetID, it.Type)
	}
	known := map[string]string{}
	if cfg.DownloadHistory && !r.Redownload {
		if m, err := backend.DownloadedPaths(keys); err != nil {
			log.Printf("job %s: reading the download history: %v", j.ID, err)
		} else {
			known = m
		}
	}
	var (
		todo      []backend.MediaItem
		todoIndex []int
		before    int
	)
	listed := make(map[string]bool, len(items))
	e.mu.Lock()
	for i, it := range items {
		path, inHistory := known[keys[i]]
		if inHistory || listed[keys[i]] {
			j.status[i], j.paths[i] = StatusDownloadedBefore, path
			before++
			continue
		}
		listed[keys[i]] = true
		todo = append(todo, it)
		todoIndex = append(todoIndex, i)
	}
	e.mu.Unlock()
	e.update(j, func(s *Job) {
		s.State = StateDownloading
		s.Total, s.Current, s.Known = len(items), before, before
	})
	if before > 0 {
		log.Printf("job %s: %d of %d items were downloaded before", j.ID, before, len(items))
	}

	// The backend reports from its worker goroutines, with indexes into
	// todo.
	var (
		last    time.Time
		records []backend.DownloadRecord
	)
	progress := func(current, total int) {
		e.mu.Lock()
		j.Current = before + current
		if j.Total > 0 {
			j.Percent = j.Current * 100 / j.Total
		}
		if current != total && time.Since(last) < progressEvery {
			e.mu.Unlock()
			return
		}
		last = time.Now()
		snap := j.Job
		e.mu.Unlock()
		e.publish(Event{Type: EventJob, Job: &snap})
	}
	itemStatus := func(index int, status, path string) {
		if index < 0 || index >= len(todoIndex) {
			return
		}
		it := todo[index]
		index = todoIndex[index]
		e.mu.Lock()
		j.status[index], j.paths[index] = status, path
		switch status {
		case "success":
			j.Downloaded++
		case "skipped":
			j.Skipped++
		case "failed":
			j.Failed++
		}
		if status == "success" || status == "skipped" {
			records = append(records, backend.DownloadRecord{
				Key: keys[index], TweetID: it.TweetID, Username: it.Username,
				MediaType: it.Type, URL: it.URL, Path: path,
			})
		}
		e.mu.Unlock()
		e.publish(Event{Type: EventItem, Item: &ItemEvent{JobID: j.ID, Index: index, Status: status, Path: path}})
	}

	options := backend.DownloadOptions{
		ConcurrentDownloads:   cfg.ConcurrentDownloads,
		SkipExistingFiles:     cfg.SkipExisting,
		DeleteIncompleteFiles: cfg.DeleteIncompleteFiles,
		RetryAttempts:         cfg.RetryAttempts,
		FilenameTemplate:      cfg.FilenameTemplate,
		FolderTemplate:        cfg.FolderTemplate,
		AutoConvertGIFs:       cfg.AutoConvertGIFs,
		GIFQuality:            cfg.GIFQuality,
		GIFResolution:         cfg.GIFResolution,
	}
	var (
		downloaded, skipped, failed int
		err                         error
	)
	if len(todo) > 0 {
		var counts backend.DownloadCounts
		counts, err = backend.DownloadItems(j.ctx, todo, outputDir, r.Username, options, cfg.Proxy,
			backend.DownloadEvents{Progress: progress, Item: itemStatus})
		downloaded, skipped, failed = counts.Downloaded, counts.Skipped, counts.Failed
	}
	e.mu.Lock()
	done := records
	e.mu.Unlock()
	if recErr := backend.RecordDownloads(done); recErr != nil {
		log.Printf("job %s: saving the download history: %v", j.ID, recErr)
	}
	e.update(j, func(s *Job) {
		s.Downloaded, s.Skipped, s.Failed = downloaded, skipped, failed
		s.Current = before + downloaded + skipped + failed
		if s.Total > 0 {
			s.Percent = s.Current * 100 / s.Total
		}
		msg := fmt.Sprintf("%d downloaded, %d already downloaded, %d failed", downloaded, skipped+before, failed)
		if len(todo) == 0 {
			msg = fmt.Sprintf("Nothing new — all %d items were downloaded before", before)
		}
		if errors.Is(err, context.Canceled) {
			msg = "Stopped — " + msg
		}
		if s.Incomplete {
			msg += " (timeline incomplete)"
		}
		s.Message = msg
	})
	return err
}

func parseSaved(raw string) (*savedTimeline, error) {
	var t savedTimeline
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return nil, fmt.Errorf("the library's data for this account is malformed: %w", err)
	}
	return &t, nil
}

func entryKey(e backend.TimelineEntry) string {
	return strconv.FormatInt(int64(e.TweetID), 10) + "|" + e.URL
}

func accountOf(a backend.AccountInfo, username string) *Account {
	name := a.Name
	if name == "" {
		name = username
	}
	return &Account{
		Name:           name,
		Nick:           a.Nick,
		ProfileImage:   a.ProfileImage,
		FollowersCount: a.FollowersCount,
		StatusesCount:  a.StatusesCount,
	}
}

// SavedItems returns the timeline the library holds for an account.
// SavedProfile returns the library's copy of a timeline with the whole
// account profile, for the app's profile page.
func SavedProfile(username, timelineType string) ([]backend.TimelineEntry, backend.AccountInfo, bool, error) {
	acc, err := backend.GetAccountByUsernameAndMediaType(username, timelineType)
	if err != nil {
		return nil, backend.AccountInfo{}, false, err
	}
	t, err := parseSaved(acc.ResponseJSON)
	if err != nil {
		return nil, backend.AccountInfo{}, false, err
	}
	return t.Timeline, t.AccountInfo, t.Completed, nil
}

func SavedItems(username, timelineType string) ([]backend.TimelineEntry, *Account, error) {
	acc, err := backend.GetAccountByUsernameAndMediaType(username, timelineType)
	if err != nil {
		return nil, nil, err
	}
	t, err := parseSaved(acc.ResponseJSON)
	if err != nil {
		return nil, nil, err
	}
	return t.Timeline, accountOf(t.AccountInfo, username), nil
}
