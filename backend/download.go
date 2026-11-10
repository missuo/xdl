package backend

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The downloader: a job's items go to the files naming.go names, a few at
// a time, each through the resumable engine (resumable.go), retried, then
// tagged with the post's link (ExifTool) and, for GIFs, converted to .gif
// (FFmpeg) when the settings ask.

// MediaItem is an item to download.
type MediaItem struct {
	URL              string `json:"url"`
	Date             string `json:"date"`
	TweetID          int64  `json:"tweet_id"`
	Type             string `json:"type"`
	Username         string `json:"username"`
	AccountName      string `json:"account_name,omitempty"`
	Content          string `json:"content,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
}

// DownloadOptions are the settings a download follows.
type DownloadOptions struct {
	ConcurrentDownloads   int
	SkipExistingFiles     bool
	DeleteIncompleteFiles bool
	RetryAttempts         int
	FilenameTemplate      string
	FolderTemplate        string
	AutoConvertGIFs       bool
	GIFQuality            string // fast, better
	GIFResolution         string // original, high, medium, low
}

const (
	defaultWorkers = 10
	maxWorkers     = 50
	maxRetries     = 5
)

func (o DownloadOptions) workers() int {
	switch {
	case o.ConcurrentDownloads <= 0:
		return defaultWorkers
	case o.ConcurrentDownloads > maxWorkers:
		return maxWorkers
	}
	return o.ConcurrentDownloads
}

func (o DownloadOptions) retries() int { return min(max(o.RetryAttempts, 0), maxRetries) }

// Item statuses a download reports.
const (
	StatusSuccess   = "success"
	StatusSkipped   = "skipped" // the file is there already
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// DownloadEvents receives a download's progress, from its goroutines.
type DownloadEvents struct {
	// Progress is called after each item, with how many are done.
	Progress func(done, total int)
	// Item is called with each item's index into the items, its status
	// and its file.
	Item func(index int, status, path string)
}

// DownloadCounts says how a download went.
type DownloadCounts struct{ Downloaded, Skipped, Failed int }

// DownloadItems downloads items into dir; handle names the folder of items
// without a Username of their own. It returns when every item is done, or
// with ctx's error when ctx ends.
func DownloadItems(ctx context.Context, items []MediaItem, dir, handle string, opts DownloadOptions, proxy string, ev DownloadEvents) (DownloadCounts, error) {
	var counts DownloadCounts
	if len(items) == 0 {
		return counts, nil
	}
	client, err := CreateHTTPClient(proxy, 0)
	if err != nil {
		return counts, err
	}
	paths := planPaths(items, dir, handle, opts, time.Now())

	var (
		downloaded, skipped, failed, done atomic.Int64
		conversions                       sync.WaitGroup
		convertSlots                      = make(chan struct{}, 2)
	)
	report := func(i int, status string) {
		switch status {
		case StatusSuccess:
			downloaded.Add(1)
		case StatusSkipped:
			skipped.Add(1)
		case StatusFailed:
			failed.Add(1)
		}
		if ev.Item != nil {
			ev.Item(i, status, paths[i])
		}
		if ev.Progress != nil {
			ev.Progress(int(done.Add(1)), len(items))
		}
	}

	work := make(chan int)
	var workers sync.WaitGroup
	for range min(opts.workers(), len(items)) {
		workers.Go(func() {
			for i := range work {
				status := downloadItem(ctx, client, items[i], paths[i], opts)
				if status == StatusSuccess && opts.AutoConvertGIFs && isGIF(items[i].Type) && FFmpeg.Installed() {
					conversions.Go(func() {
						convertSlots <- struct{}{}
						defer func() { <-convertSlots }()
						out := strings.TrimSuffix(paths[i], filepath.Ext(paths[i])) + ".gif"
						_ = ConvertMP4ToGIF(ctx, paths[i], out, opts.GIFQuality, opts.GIFResolution)
					})
				}
				report(i, status)
			}
		})
	}
	for i := range items {
		if ctx.Err() != nil {
			report(i, StatusCancelled)
			continue
		}
		work <- i
	}
	close(work)
	workers.Wait()
	conversions.Wait()

	counts = DownloadCounts{int(downloaded.Load()), int(skipped.Load()), int(failed.Load())}
	return counts, ctx.Err()
}

func isGIF(kind string) bool { return kind == "animated_gif" || kind == "gif" }

func downloadItem(ctx context.Context, client *http.Client, it MediaItem, path string, opts DownloadOptions) string {
	if ctx.Err() != nil {
		return StatusCancelled
	}
	if opts.SkipExistingFiles && fileExists(path) {
		return StatusSkipped
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return StatusFailed
	}
	if it.Type == "text" {
		if writeFileAtomically(path, []byte(it.Content)) != nil {
			return StatusFailed
		}
		return StatusSuccess
	}
	if err := downloadFileWithRetry(ctx, client, it.URL, path, opts); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return StatusCancelled
		}
		return StatusFailed
	}
	original := firstNonEmpty(it.OriginalFilename, ExtractOriginalFilename(it.URL))
	_ = writeComment(ctx, path, "https://x.com/i/status/"+itoa(it.TweetID), original)
	return StatusSuccess
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// planPaths names the file of each item; items that would share a name in
// a folder are numbered _01, _02… in their order.
func planPaths(items []MediaItem, dir, handle string, opts DownloadOptions, now time.Time) []string {
	type spot struct{ folder, base, ext string }
	spots := make([]spot, len(items))
	inPost := map[string]int{} // the items of each post so far, by account and post
	uses := map[string]int{}
	for i, it := range items {
		who := firstNonEmpty(it.Username, handle)
		name := firstNonEmpty(it.AccountName, who)
		post := who + "\x00" + itoa(it.TweetID)
		inPost[post]++
		s := spot{
			folder: filepath.Join(dir, folderName(opts.FolderTemplate, who, name, now), kindFolder(it.Type)),
			base:   fileName(opts.FilenameTemplate, it, who, name, inPost[post]),
			ext:    fileExtension(it.URL, it.Type),
		}
		spots[i] = s
		uses[s.folder+"\x00"+s.base+s.ext]++
	}
	paths := make([]string, len(items))
	nth := map[string]int{}
	for i, s := range spots {
		key := s.folder + "\x00" + s.base + s.ext
		name := s.base + s.ext
		if uses[key] > 1 {
			nth[key]++
			name = s.base + "_" + twoDigits(nth[key]) + s.ext
		}
		paths[i] = filepath.Join(s.folder, name)
	}
	return paths
}

// downloadFileWithRetry downloads a file, each retry continuing where the
// last try stopped. A stop keeps what came, for the next run to continue;
// a file that fails every try loses it when the options say so.
func downloadFileWithRetry(ctx context.Context, client *http.Client, url, path string, opts DownloadOptions) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	var err error
	for attempt := 0; attempt <= opts.retries(); attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = fetchToPart(ctx, client, url, path); err == nil {
			if err = ctx.Err(); err != nil {
				return err
			}
			return os.Rename(partPath(path), path)
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return err
		}
	}
	if opts.DeleteIncompleteFiles {
		removeParts(path)
	}
	return err
}

// writeFileAtomically writes data to path through a temporary file, so the
// file is whole or not there.
func writeFileAtomically(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
