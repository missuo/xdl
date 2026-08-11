package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"xdl/backend"
)

type apiServer struct {
	secret string
	events *eventBuffer

	downloadMu     sync.Mutex
	downloadCancel context.CancelFunc
	downloadActive bool

	busyMu   sync.Mutex
	busyJobs map[string]bool // dep downloads / gif conversions in flight

	settingsMu sync.Mutex
	settings   appSettings

	jobsMu   sync.Mutex
	jobQueue []string
	jobs     chan string
}

func newAPIServer(secret string) *apiServer {
	return &apiServer{
		secret:   secret,
		events:   newEventBuffer(4096),
		busyJobs: make(map[string]bool),
		settings: defaultAppSettings(),
		jobs:     make(chan string, 64),
	}
}

func (a *apiServer) serve(listener net.Listener, shutdown chan struct{}) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{"ok": true, "version": coreVersion})
	}))
	mux.HandleFunc("GET /events", a.auth(a.handleEvents))
	mux.HandleFunc("POST /shutdown", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() {
			time.Sleep(100 * time.Millisecond)
			close(shutdown)
		}()
	}))

	mux.HandleFunc("POST /extract/timeline", a.auth(a.handleExtractTimeline))
	mux.HandleFunc("POST /extract/daterange", a.auth(a.handleExtractDateRange))
	mux.HandleFunc("POST /extract/cancel", a.auth(func(w http.ResponseWriter, r *http.Request) {
		backend.KillAllExtractorProcesses()
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("POST /download", a.auth(a.handleDownload))
	mux.HandleFunc("POST /download/plan", a.auth(a.handleDownloadPlan))
	mux.HandleFunc("POST /download/stop", a.auth(a.handleDownloadStop))
	mux.HandleFunc("GET /download/state", a.auth(func(w http.ResponseWriter, r *http.Request) {
		a.downloadMu.Lock()
		active := a.downloadActive
		a.downloadMu.Unlock()
		writeJSON(w, map[string]bool{"active": active})
	}))
	mux.HandleFunc("POST /download/profile-image", a.auth(a.handleProfileImage))

	mux.HandleFunc("GET /defaults", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"download_path": backend.GetDefaultDownloadPath()})
	}))

	mux.HandleFunc("GET /settings", a.auth(a.handleSettingsGet))
	mux.HandleFunc("POST /settings", a.auth(a.handleSettingsSet))

	mux.HandleFunc("GET /authtoken", a.auth(a.handleAuthTokenGet))
	mux.HandleFunc("POST /authtoken", a.auth(a.handleAuthTokenSet))

	mux.HandleFunc("GET /deps/installed", a.auth(a.handleDepsInstalled))
	mux.HandleFunc("GET /deps/status", a.auth(a.handleDepsStatus))
	mux.HandleFunc("POST /deps/download", a.auth(a.handleDepsDownload))

	mux.HandleFunc("GET /accounts", a.auth(a.handleAccountsList))
	mux.HandleFunc("POST /accounts/save", a.auth(a.handleAccountSave))
	mux.HandleFunc("GET /accounts/get", a.auth(a.handleAccountGet))
	mux.HandleFunc("GET /accounts/saved", a.auth(a.handleAccountSaved))
	mux.HandleFunc("POST /accounts/delete", a.auth(a.handleAccountDelete))
	mux.HandleFunc("POST /accounts/clear", a.auth(func(w http.ResponseWriter, r *http.Request) {
		writeResult(w, backend.ClearAllAccounts())
	}))
	mux.HandleFunc("POST /accounts/group", a.auth(a.handleAccountGroup))
	mux.HandleFunc("GET /groups", a.auth(func(w http.ResponseWriter, r *http.Request) {
		groups, err := backend.GetAllGroups()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, groups)
	}))
	mux.HandleFunc("POST /accounts/export-json", a.auth(a.handleExportJSON))
	mux.HandleFunc("POST /accounts/export-txt", a.auth(a.handleExportTXT))
	mux.HandleFunc("POST /accounts/import", a.auth(a.handleImport))

	mux.HandleFunc("POST /fs/check-folders", a.auth(a.handleCheckFolders))
	mux.HandleFunc("POST /fs/gifs-info", a.auth(a.handleGifsInfo))
	mux.HandleFunc("POST /gifs/convert", a.auth(a.handleGifsConvert))

	go a.runBrowserJobs()
	go a.serveBrowserBridge(shutdown)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		// No WriteTimeout: extraction responses can take minutes.
	}
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("api server: %v", err)
	}
}

func (a *apiServer) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// --- extraction ---

func (a *apiServer) handleExtractTimeline(w http.ResponseWriter, r *http.Request) {
	var req backend.TimelineRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Username == "" && req.TimelineType != "bookmarks" {
		writeError(w, http.StatusBadRequest, errors.New("username is required"))
		return
	}
	if req.AuthToken == "" {
		writeError(w, http.StatusBadRequest, errors.New("auth token is required"))
		return
	}
	resp, err := backend.ExtractTimeline(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, resp)
}

func (a *apiServer) handleExtractDateRange(w http.ResponseWriter, r *http.Request) {
	var req backend.DateRangeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Username == "" || req.AuthToken == "" || req.StartDate == "" || req.EndDate == "" {
		writeError(w, http.StatusBadRequest, errors.New("username, auth_token, start_date and end_date are required"))
		return
	}
	resp, err := backend.ExtractDateRange(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, resp)
}

// --- download ---

type mediaItemRequest struct {
	URL              string                `json:"url"`
	Date             string                `json:"date"`
	TweetID          backend.TweetIDString `json:"tweet_id"`
	Type             string                `json:"type"`
	Content          string                `json:"content,omitempty"`
	OriginalFilename string                `json:"original_filename,omitempty"`
	AuthorUsername   string                `json:"author_username,omitempty"`
	AccountName      string                `json:"account_name,omitempty"`
}

type downloadRequest struct {
	Items                 []mediaItemRequest `json:"items"`
	OutputDir             string             `json:"output_dir"`
	Username              string             `json:"username"`
	ConcurrentDownloads   int                `json:"concurrent_downloads,omitempty"`
	SkipExisting          bool               `json:"skip_existing"`
	DeleteIncompleteFiles bool               `json:"delete_incomplete_files"`
	RetryAttempts         int                `json:"retry_attempts,omitempty"`
	Proxy                 string             `json:"proxy,omitempty"`
	FilenameTemplate      string             `json:"filename_template,omitempty"`
	FolderTemplate        string             `json:"folder_template,omitempty"`
	AutoConvertGIFs       bool               `json:"auto_convert_gifs,omitempty"`
	GIFQuality            string             `json:"gif_quality,omitempty"`
	GIFResolution         string             `json:"gif_resolution,omitempty"`
}

type downloadResult struct {
	Success    bool   `json:"success"`
	Cancelled  bool   `json:"cancelled"`
	Downloaded int    `json:"downloaded"`
	Skipped    int    `json:"skipped"`
	Failed     int    `json:"failed"`
	Message    string `json:"message"`
}

func (a *apiServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	var req downloadRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("no items provided"))
		return
	}

	a.downloadMu.Lock()
	if a.downloadActive {
		a.downloadMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("another download is already in progress"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloadCancel = cancel
	a.downloadActive = true
	a.downloadMu.Unlock()

	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = backend.GetDefaultDownloadPath()
	}

	items := buildMediaItems(req)

	options := backend.DownloadOptions{
		ConcurrentDownloads:   req.ConcurrentDownloads,
		SkipExistingFiles:     req.SkipExisting,
		DeleteIncompleteFiles: req.DeleteIncompleteFiles,
		RetryAttempts:         req.RetryAttempts,
		FilenameTemplate:      req.FilenameTemplate,
		FolderTemplate:        req.FolderTemplate,
		AutoConvertGIFs:       req.AutoConvertGIFs,
		GIFQuality:            req.GIFQuality,
		GIFResolution:         req.GIFResolution,
	}

	go func() {
		defer func() {
			a.downloadMu.Lock()
			a.downloadCancel = nil
			a.downloadActive = false
			a.downloadMu.Unlock()
		}()

		progress := func(current, total int) {
			percent := 0
			if total > 0 {
				percent = current * 100 / total
			}
			a.events.emit("download-progress", map[string]int{
				"current": current, "total": total, "percent": percent,
			})
		}
		// tweet_id crosses as a string everywhere (the old app mixed number
		// and string and the frontend had to normalize).
		itemStatus := func(tweetID int64, index int, status string, path string) {
			a.events.emit("download-item-status", map[string]interface{}{
				"tweet_id": strconv.FormatInt(tweetID, 10),
				"index":    index,
				"status":   status,
				"path":     path,
			})
		}

		downloaded, skipped, failed, err := backend.DownloadMediaWithMetadataProgressAndStatus(
			items, outputDir, req.Username, progress, itemStatus, ctx, options, req.Proxy)

		result := downloadResult{
			Downloaded: downloaded, Skipped: skipped, Failed: failed,
		}
		switch {
		case err != nil && errors.Is(err, context.Canceled):
			result.Cancelled = true
			result.Message = "Download stopped"
		case err != nil:
			result.Message = err.Error()
		default:
			result.Success = true
			result.Message = fmt.Sprintf("Downloaded %d files, %d skipped, %d failed", downloaded, skipped, failed)
		}
		a.events.emit("download-finished", result)
	}()

	writeJSON(w, map[string]interface{}{"started": true, "total": len(items)})
}

func buildMediaItems(req downloadRequest) []backend.MediaItem {
	items := make([]backend.MediaItem, len(req.Items))
	for i, item := range req.Items {
		originalFilename := item.OriginalFilename
		if originalFilename == "" {
			originalFilename = backend.ExtractOriginalFilename(item.URL)
		}
		username := req.Username
		if (req.Username == "bookmarks" || req.Username == "likes") && item.AuthorUsername != "" {
			username = item.AuthorUsername
		}
		items[i] = backend.MediaItem{
			URL:              item.URL,
			Date:             item.Date,
			TweetID:          int64(item.TweetID),
			Type:             item.Type,
			Username:         username,
			AccountName:      item.AccountName,
			Content:          item.Content,
			OriginalFilename: originalFilename,
		}
	}
	return items
}

// handleDownloadPlan is a dry run of /download: it reports where each item
// would land and whether that file already exists locally.
func (a *apiServer) handleDownloadPlan(w http.ResponseWriter, r *http.Request) {
	var req downloadRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("no items provided"))
		return
	}
	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = backend.GetDefaultDownloadPath()
	}
	options := backend.DownloadOptions{
		FilenameTemplate: req.FilenameTemplate,
		FolderTemplate:   req.FolderTemplate,
	}
	planned := backend.PlanMediaPaths(buildMediaItems(req), outputDir, req.Username, options)
	existing := 0
	for _, plan := range planned {
		if plan.Exists {
			existing++
		}
	}
	writeJSON(w, map[string]interface{}{
		"items":    planned,
		"existing": existing,
		"total":    len(req.Items),
	})
}

func (a *apiServer) handleDownloadStop(w http.ResponseWriter, r *http.Request) {
	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()
	if !a.downloadActive || a.downloadCancel == nil {
		writeJSON(w, map[string]bool{"stopped": false})
		return
	}
	a.downloadCancel()
	writeJSON(w, map[string]bool{"stopped": true})
}

func (a *apiServer) handleProfileImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL       string `json:"url"`
		OutputDir string `json:"output_dir"`
		Username  string `json:"username"`
		Kind      string `json:"kind"`
		Proxy     string `json:"proxy,omitempty"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, errors.New("image url is required"))
		return
	}
	outputDir := req.OutputDir
	if outputDir == "" {
		outputDir = backend.GetDefaultDownloadPath()
	}
	path, err := backend.DownloadProfileImage(req.URL, outputDir, req.Username, req.Kind, req.Proxy)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

// --- auth token (keychain) ---

func (a *apiServer) handleAuthTokenGet(w http.ResponseWriter, r *http.Request) {
	token, err := backend.GetStoredAuthToken(r.URL.Query().Get("slot"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"token": token})
}

func (a *apiServer) handleAuthTokenSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slot  string `json:"slot"`
		Token string `json:"token"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeResult(w, backend.SetStoredAuthToken(req.Slot, req.Token))
}

// --- dependencies ---

func (a *apiServer) handleDepsInstalled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{
		"xtractor": backend.IsExtractorInstalled(),
		"ffmpeg":   backend.IsFFmpegInstalled(),
		"exiftool": backend.IsExifToolInstalled(),
	})
}

// handleDepsStatus hits the network for latest versions; the app calls it
// only from the Dependencies page.
func (a *apiServer) handleDepsStatus(w http.ResponseWriter, r *http.Request) {
	type result struct {
		key    string
		status interface{}
	}
	results := make(chan result, 3)
	go func() { results <- result{"xtractor", backend.GetExtractorVersionStatus()} }()
	go func() { results <- result{"ffmpeg", backend.GetFFmpegVersionStatus()} }()
	go func() { results <- result{"exiftool", backend.GetExifToolVersionStatus()} }()
	out := make(map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		res := <-results
		out[res.key] = res.status
	}
	writeJSON(w, out)
}

func (a *apiServer) handleDepsDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	var download func(func(int64, int64)) error
	switch req.Name {
	case "xtractor":
		download = backend.DownloadExtractor
	case "ffmpeg":
		download = backend.DownloadFFmpeg
	case "exiftool":
		download = backend.DownloadExifTool
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown dependency %q", req.Name))
		return
	}
	if !a.claimJob("dep:" + req.Name) {
		writeError(w, http.StatusConflict, fmt.Errorf("%s download already in progress", req.Name))
		return
	}
	name := req.Name
	go func() {
		defer a.releaseJob("dep:" + name)
		lastEmit := time.Time{}
		err := download(func(downloaded, total int64) {
			if time.Since(lastEmit) < 200*time.Millisecond && downloaded != total {
				return
			}
			lastEmit = time.Now()
			a.events.emit("dep-progress", map[string]interface{}{
				"name": name, "downloaded": downloaded, "total": total,
			})
		})
		payload := map[string]interface{}{"name": name}
		if err != nil {
			payload["error"] = err.Error()
		}
		a.events.emit("dep-finished", payload)
	}()
	writeJSON(w, map[string]bool{"started": true})
}

func (a *apiServer) claimJob(key string) bool {
	a.busyMu.Lock()
	defer a.busyMu.Unlock()
	if a.busyJobs[key] {
		return false
	}
	a.busyJobs[key] = true
	return true
}

func (a *apiServer) releaseJob(key string) {
	a.busyMu.Lock()
	defer a.busyMu.Unlock()
	delete(a.busyJobs, key)
}

// --- accounts / database ---

func (a *apiServer) handleAccountsList(w http.ResponseWriter, r *http.Request) {
	accounts, err := backend.GetAllAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, accounts)
}

func (a *apiServer) handleAccountSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username     string `json:"username"`
		Name         string `json:"name"`
		ProfileImage string `json:"profile_image"`
		TotalMedia   int    `json:"total_media"`
		ResponseJSON string `json:"response_json"`
		MediaType    string `json:"media_type"`
		Cursor       string `json:"cursor"`
		Completed    *bool  `json:"completed"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	completed := true
	if req.Completed != nil {
		completed = *req.Completed
	}
	writeResult(w, backend.SaveAccountWithStatus(req.Username, req.Name, req.ProfileImage,
		req.TotalMedia, req.ResponseJSON, req.MediaType, req.Cursor, completed))
}

func (a *apiServer) handleAccountGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}
	acc, err := backend.GetAccountByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeRawJSONField(w, acc.ResponseJSON)
}

func (a *apiServer) handleAccountSaved(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	acc, err := backend.GetAccountByUsernameAndMediaType(q.Get("username"), q.Get("media_type"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeRawJSONField(w, acc.ResponseJSON)
}

func (a *apiServer) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeResult(w, backend.DeleteAccount(req.ID))
}

func (a *apiServer) handleAccountGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         int64  `json:"id"`
		GroupName  string `json:"group_name"`
		GroupColor string `json:"group_color"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeResult(w, backend.UpdateAccountGroup(req.ID, req.GroupName, req.GroupColor))
}

func (a *apiServer) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        int64  `json:"id"`
		OutputDir string `json:"output_dir"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	path, err := backend.ExportAccountToFile(req.ID, req.OutputDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

func (a *apiServer) handleExportTXT(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs       []int64 `json:"ids"`
		OutputDir string  `json:"output_dir"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	path, err := backend.ExportAccountsToTXT(req.IDs, req.OutputDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"path": path})
}

func (a *apiServer) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	username, err := backend.ImportAccountFromFile(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]string{"username": username})
}

// --- filesystem / gifs ---

func (a *apiServer) handleCheckFolders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base      string   `json:"base"`
		Usernames []string `json:"usernames"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, backend.CheckFoldersExist(req.Base, req.Usernames))
}

func (a *apiServer) handleGifsInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base     string `json:"base"`
		Username string `json:"username"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, map[string]interface{}{
		"exists":  backend.CheckGifsFolderExists(req.Base, req.Username),
		"has_mp4": backend.CheckGifsFolderHasMP4(req.Base, req.Username),
		"path":    backend.GetGifsFolderPath(req.Base, req.Username),
	})
}

func (a *apiServer) handleGifsConvert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FolderPath     string `json:"folder_path"`
		Quality        string `json:"quality"`
		Resolution     string `json:"resolution"`
		DeleteOriginal bool   `json:"delete_original"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if !backend.IsFFmpegInstalled() {
		writeError(w, http.StatusPreconditionFailed, errors.New("FFmpeg not installed"))
		return
	}
	if req.Quality == "" {
		req.Quality = "fast"
	}
	if req.Resolution == "" {
		req.Resolution = "high"
	}
	if !a.claimJob("gifs") {
		writeError(w, http.StatusConflict, errors.New("a GIF conversion is already in progress"))
		return
	}
	go func() {
		defer a.releaseJob("gifs")
		converted, failed, err := backend.ConvertGIFsInFolder(req.FolderPath, req.Quality, req.Resolution, req.DeleteOriginal)
		payload := map[string]interface{}{"converted": converted, "failed": failed}
		if err != nil {
			payload["error"] = err.Error()
		}
		a.events.emit("gifs-finished", payload)
	}()
	writeJSON(w, map[string]bool{"started": true})
}

// --- events ---

func (a *apiServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	cursor := int64(-1)
	if c := r.URL.Query().Get("cursor"); c != "" {
		if parsed, err := strconv.ParseInt(c, 10, 64); err == nil {
			cursor = parsed
		}
	}
	data, err := a.events.marshalSince(cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// --- helpers ---

func decodeBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// writeRawJSONField wraps an already-JSON string (the stored response_json)
// without re-encoding it as a quoted string.
func writeRawJSONField(w http.ResponseWriter, raw string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"response_json":%s}`, raw)
}

func writeResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
