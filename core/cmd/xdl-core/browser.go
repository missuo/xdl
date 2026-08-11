package main

// Browser bridge: a second, fixed-port localhost listener for the XDL browser
// extension. The main control API uses an ephemeral port + secret that an
// extension cannot discover, so this listener exists solely to accept
// "download this profile" requests. It only trusts requests whose Origin is a
// browser extension — regular web pages cannot forge that header.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"xdl/backend"
)

const browserBridgeAddr = "127.0.0.1:47333"

// appSettings mirrors the GUI's download preferences; the app syncs them via
// POST /settings so browser-initiated jobs behave exactly like GUI downloads.
type appSettings struct {
	DownloadPath          string `json:"download_path"`
	ConcurrentDownloads   int    `json:"concurrent_downloads"`
	SkipExisting          bool   `json:"skip_existing"`
	DeleteIncompleteFiles bool   `json:"delete_incomplete_files"`
	RetryAttempts         int    `json:"retry_attempts"`
	Proxy                 string `json:"proxy"`
	FilenameTemplate      string `json:"filename_template"`
	FolderTemplate        string `json:"folder_template"`
	AutoConvertGIFs       bool   `json:"auto_convert_gifs"`
	GIFQuality            string `json:"gif_quality"`
	GIFResolution         string `json:"gif_resolution"`
	IncludePhotos         bool   `json:"include_photos"`
	IncludeVideos         bool   `json:"include_videos"`
	IncludeGifs           bool   `json:"include_gifs"`
	IncludeText           bool   `json:"include_text"`
	Retweets              bool   `json:"retweets"`
	VideoQuality          string `json:"video_quality"`
	ImageSize             string `json:"image_size"`
}

func defaultAppSettings() appSettings {
	return appSettings{
		ConcurrentDownloads:   10,
		SkipExisting:          true,
		DeleteIncompleteFiles: true,
		RetryAttempts:         2,
		FilenameTemplate:      "{handle}_{date}_{tweet_id}",
		FolderTemplate:        "{handle}",
		GIFQuality:            "fast",
		GIFResolution:         "original",
		IncludePhotos:         true,
		IncludeVideos:         true,
		IncludeGifs:           true,
		VideoQuality:          "highest",
		ImageSize:             "orig",
	}
}

func (a *apiServer) snapshotSettings() appSettings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	return a.settings
}

func (a *apiServer) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.snapshotSettings())
}

func (a *apiServer) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	settings := defaultAppSettings()
	if !decodeBody(w, r, &settings) {
		return
	}
	a.settingsMu.Lock()
	a.settings = settings
	a.settingsMu.Unlock()
	writeJSON(w, map[string]bool{"ok": true})
}

// --- bridge listener ---

func (a *apiServer) serveBrowserBridge(shutdown chan struct{}) {
	mux := http.NewServeMux()
	mux.HandleFunc("/enqueue", a.bridgeCORS(a.handleBridgeEnqueue))
	mux.HandleFunc("/status", a.bridgeCORS(a.handleBridgeStatus))

	server := &http.Server{
		Addr:              browserBridgeAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go func() {
		<-shutdown
		server.Close()
	}()
	log.Printf("browser bridge listening on %s", browserBridgeAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("browser bridge unavailable: %v", err)
	}
}

func trustedBridgeOrigin(origin string) bool {
	return strings.HasPrefix(origin, "chrome-extension://") ||
		strings.HasPrefix(origin, "moz-extension://") ||
		strings.HasPrefix(origin, "safari-web-extension://")
}

func (a *apiServer) bridgeCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !trustedBridgeOrigin(origin) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (a *apiServer) handleBridgeStatus(w http.ResponseWriter, r *http.Request) {
	a.jobsMu.Lock()
	queue := append([]string(nil), a.jobQueue...)
	a.jobsMu.Unlock()
	writeJSON(w, map[string]interface{}{
		"app":     "XDL",
		"version": coreVersion,
		"queue":   queue,
	})
}

func (a *apiServer) handleBridgeEnqueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	username := strings.TrimPrefix(strings.TrimSpace(req.Username), "@")
	if username == "" || len(username) > 15 {
		writeError(w, http.StatusBadRequest, errors.New("invalid username"))
		return
	}

	a.jobsMu.Lock()
	alreadyQueued := false
	for _, queued := range a.jobQueue {
		if strings.EqualFold(queued, username) {
			alreadyQueued = true
			break
		}
	}
	if !alreadyQueued {
		a.jobQueue = append(a.jobQueue, username)
	}
	position := len(a.jobQueue)
	a.jobsMu.Unlock()

	if alreadyQueued {
		writeJSON(w, map[string]interface{}{"queued": true, "duplicate": true})
		return
	}

	select {
	case a.jobs <- username:
		a.events.emit("browser-job", map[string]interface{}{
			"username": username, "stage": "queued", "position": position,
		})
		writeJSON(w, map[string]interface{}{"queued": true, "position": position})
	default:
		a.jobsMu.Lock()
		if n := len(a.jobQueue); n > 0 && strings.EqualFold(a.jobQueue[n-1], username) {
			a.jobQueue = a.jobQueue[:n-1]
		}
		a.jobsMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, errors.New("job queue is full"))
	}
}

// --- job runner ---

func (a *apiServer) runBrowserJobs() {
	for username := range a.jobs {
		a.runBrowserJob(username)
		a.jobsMu.Lock()
		for i, queued := range a.jobQueue {
			if strings.EqualFold(queued, username) {
				a.jobQueue = append(a.jobQueue[:i], a.jobQueue[i+1:]...)
				break
			}
		}
		a.jobsMu.Unlock()
	}
}

func (a *apiServer) jobEvent(username, stage string, extra map[string]interface{}) {
	payload := map[string]interface{}{"username": username, "stage": stage}
	for key, value := range extra {
		payload[key] = value
	}
	a.events.emit("browser-job", payload)
}

func (a *apiServer) runBrowserJob(username string) {
	settings := a.snapshotSettings()

	token, err := backend.GetStoredAuthToken("public")
	if err != nil || token == "" {
		a.jobEvent(username, "error", map[string]interface{}{
			"message": "no auth token stored — save one in the app first",
		})
		return
	}

	// Fetch all pages.
	a.jobEvent(username, "fetching", nil)
	var timeline []backend.TimelineEntry
	var account backend.AccountInfo
	cursor := ""
	completed := false
	for page := 1; page <= 500; page++ {
		resp, err := backend.ExtractTimeline(backend.TimelineRequest{
			Username:      username,
			AuthToken:     token,
			TimelineType:  "media",
			BatchSize:     100,
			Page:          page,
			MediaType:     "media",
			Retweets:      settings.Retweets,
			Cursor:        cursor,
			IncludePhotos: settings.IncludePhotos,
			IncludeVideos: settings.IncludeVideos,
			IncludeGifs:   settings.IncludeGifs,
			IncludeText:   settings.IncludeText,
			VideoQuality:  settings.VideoQuality,
			ImageSize:     settings.ImageSize,
		})
		if err != nil {
			if len(timeline) == 0 {
				a.jobEvent(username, "error", map[string]interface{}{"message": err.Error()})
				return
			}
			break
		}
		account = resp.AccountInfo
		before := len(timeline)
		timeline = append(timeline, resp.Timeline...)
		a.jobEvent(username, "fetching", map[string]interface{}{
			"pages": page, "items": len(timeline),
		})

		nextCursor := resp.Metadata.Cursor
		if nextCursor == "" {
			nextCursor = resp.Cursor
		}
		if !resp.Metadata.HasMore {
			completed = true
			cursor = nextCursor
			break
		}
		if len(timeline) == before || nextCursor == "" || nextCursor == cursor {
			break
		}
		cursor = nextCursor
	}
	if len(timeline) == 0 {
		a.jobEvent(username, "error", map[string]interface{}{"message": "no media found"})
		return
	}

	// Save to the library, exactly like a GUI fetch would.
	merged := map[string]interface{}{
		"account_info": account,
		"total_urls":   len(timeline),
		"timeline":     timeline,
		"metadata":     map[string]interface{}{"has_more": !completed, "cursor": cursor},
		"cursor":       cursor,
		"completed":    completed,
	}
	if data, err := json.Marshal(merged); err == nil {
		if err := backend.SaveAccountWithStatus(username, account.Nick, account.ProfileImage,
			len(timeline), string(data), "media", cursor, completed); err == nil {
			a.events.emit("accounts-changed", map[string]interface{}{"username": username})
		}
	}

	// Download through the shared single-download slot so it can't collide
	// with a GUI download; /download/stop cancels this too.
	items := make([]backend.MediaItem, len(timeline))
	for i, entry := range timeline {
		items[i] = backend.MediaItem{
			URL:              entry.URL,
			Date:             entry.Date,
			TweetID:          int64(entry.TweetID),
			Type:             entry.Type,
			Username:         username,
			AccountName:      account.Nick,
			Content:          entry.Content,
			OriginalFilename: entry.OriginalFilename,
		}
	}

	ctx, err := a.claimDownloadSlot(30 * time.Minute)
	if err != nil {
		a.jobEvent(username, "error", map[string]interface{}{"message": err.Error()})
		return
	}
	defer a.releaseDownloadSlot()

	outputDir := settings.DownloadPath
	if outputDir == "" {
		outputDir = backend.GetDefaultDownloadPath()
	}
	options := backend.DownloadOptions{
		ConcurrentDownloads:   settings.ConcurrentDownloads,
		SkipExistingFiles:     settings.SkipExisting,
		DeleteIncompleteFiles: settings.DeleteIncompleteFiles,
		RetryAttempts:         settings.RetryAttempts,
		FilenameTemplate:      settings.FilenameTemplate,
		FolderTemplate:        settings.FolderTemplate,
		AutoConvertGIFs:       settings.AutoConvertGIFs,
		GIFQuality:            settings.GIFQuality,
		GIFResolution:         settings.GIFResolution,
	}

	a.jobEvent(username, "downloading", map[string]interface{}{"total": len(items)})
	lastEmit := time.Time{}
	progress := func(current, total int) {
		if time.Since(lastEmit) < 200*time.Millisecond && current != total {
			return
		}
		lastEmit = time.Now()
		a.events.emit("browser-job-progress", map[string]interface{}{
			"username": username, "current": current, "total": total,
		})
	}

	downloaded, skipped, failed, err := backend.DownloadMediaWithMetadataProgressAndStatus(
		items, outputDir, username, progress, nil, ctx, options, settings.Proxy)

	result := map[string]interface{}{
		"downloaded": downloaded, "skipped": skipped, "failed": failed, "total": len(items),
	}
	if err != nil {
		result["message"] = err.Error()
		a.jobEvent(username, "error", result)
		return
	}
	result["message"] = fmt.Sprintf("Downloaded %d files, %d skipped, %d failed", downloaded, skipped, failed)
	a.jobEvent(username, "done", result)
}

// claimDownloadSlot waits for the single-download slot used by /download.
func (a *apiServer) claimDownloadSlot(maxWait time.Duration) (context.Context, error) {
	deadline := time.Now().Add(maxWait)
	for {
		a.downloadMu.Lock()
		if !a.downloadActive {
			ctx, cancel := context.WithCancel(context.Background())
			a.downloadCancel = cancel
			a.downloadActive = true
			a.downloadMu.Unlock()
			return ctx, nil
		}
		a.downloadMu.Unlock()
		if time.Now().After(deadline) {
			return nil, errors.New("timed out waiting for the download slot")
		}
		time.Sleep(2 * time.Second)
	}
}

func (a *apiServer) releaseDownloadSlot() {
	a.downloadMu.Lock()
	a.downloadCancel = nil
	a.downloadActive = false
	a.downloadMu.Unlock()
}
