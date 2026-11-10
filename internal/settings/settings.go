// Package settings persists XDL's preferences in a JSON file shared by the
// GUI, the CLI and the headless server, so a job behaves the same whichever
// of them started it.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/missuo/xdl/backend"
)

// Settings are the user's download and fetch preferences.
type Settings struct {
	DownloadPath          string `json:"download_path"` // empty = backend default (~/Pictures)
	ConcurrentDownloads   int    `json:"concurrent_downloads"`
	RetryAttempts         int    `json:"retry_attempts"`
	SkipExisting          bool   `json:"skip_existing"`
	DeleteIncompleteFiles bool   `json:"delete_incomplete_files"`
	Proxy                 string `json:"proxy"`
	FetchTimeout          int    `json:"fetch_timeout"` // seconds per extraction page

	IncludePhotos bool   `json:"include_photos"`
	IncludeVideos bool   `json:"include_videos"`
	IncludeGifs   bool   `json:"include_gifs"`
	IncludeText   bool   `json:"include_text"`
	Retweets      bool   `json:"retweets"`
	VideoQuality  string `json:"video_quality"` // highest|720|480|360|lowest
	ImageSize     string `json:"image_size"`    // orig|large|medium|small

	FilenameTemplate string `json:"filename_template"`
	FolderTemplate   string `json:"folder_template"`

	AutoConvertGIFs bool   `json:"auto_convert_gifs"`
	GIFQuality      string `json:"gif_quality"`    // fast|better
	GIFResolution   string `json:"gif_resolution"` // original|high|medium|low

	// Incremental refetches stop paging once a page brings nothing new for
	// an account whose previous fetch completed, and merge with it.
	Incremental bool `json:"incremental"`
	// DownloadHistory skips media downloaded before, by its X media ID,
	// even after its file was renamed, moved or deleted.
	DownloadHistory bool `json:"download_history"`
	// Notify shows a desktop notification when a job finishes (GUI only).
	Notify bool `json:"notify"`
}

// Defaults returns the factory settings (the same as the Objective-C app's).
func Defaults() Settings {
	return Settings{
		ConcurrentDownloads:   10,
		RetryAttempts:         2,
		SkipExisting:          true,
		DeleteIncompleteFiles: true,
		FetchTimeout:          60,
		IncludePhotos:         true,
		IncludeVideos:         true,
		IncludeGifs:           true,
		VideoQuality:          "highest",
		ImageSize:             "orig",
		FilenameTemplate:      "{handle}_{date}_{tweet_id}",
		FolderTemplate:        "{handle}",
		GIFQuality:            "fast",
		GIFResolution:         "original",
		Incremental:           true,
		DownloadHistory:       true,
		Notify:                true,
	}
}

// Option values accepted by the enumerated settings.
var (
	VideoQualities = []string{"highest", "720", "480", "360", "lowest"}
	ImageSizes     = []string{"orig", "large", "medium", "small"}
	GIFQualities   = []string{"fast", "better"}
	GIFResolutions = []string{"original", "high", "medium", "low"}
)

// Normalize clamps numbers and replaces unknown enum values with defaults.
func (s Settings) Normalize() Settings {
	d := Defaults()
	s.ConcurrentDownloads = clamp(s.ConcurrentDownloads, 1, 50, d.ConcurrentDownloads)
	s.RetryAttempts = clamp(s.RetryAttempts, 1, 5, d.RetryAttempts)
	s.FetchTimeout = clamp(s.FetchTimeout, 10, 600, d.FetchTimeout)
	s.VideoQuality = oneOf(s.VideoQuality, VideoQualities, d.VideoQuality)
	s.ImageSize = oneOf(s.ImageSize, ImageSizes, d.ImageSize)
	s.GIFQuality = oneOf(s.GIFQuality, GIFQualities, d.GIFQuality)
	s.GIFResolution = oneOf(s.GIFResolution, GIFResolutions, d.GIFResolution)
	if strings.TrimSpace(s.FilenameTemplate) == "" {
		s.FilenameTemplate = d.FilenameTemplate
	}
	if strings.TrimSpace(s.FolderTemplate) == "" {
		s.FolderTemplate = d.FolderTemplate
	}
	s.DownloadPath = strings.TrimSpace(s.DownloadPath)
	s.Proxy = strings.TrimSpace(s.Proxy)
	return s
}

func clamp(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	return max(lo, min(hi, v))
}

func oneOf(v string, allowed []string, def string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return def
}

// DownloadDir is where downloads go: the setting, else XDL_DOWNLOAD_DIR
// (for containers), else ~/Pictures.
func (s Settings) DownloadDir() string {
	if s.DownloadPath != "" {
		return s.DownloadPath
	}
	if dir := os.Getenv("XDL_DOWNLOAD_DIR"); dir != "" {
		return dir
	}
	return backend.GetDefaultDownloadPath()
}

// Dir is XDL's data directory (~/Library/Application Support/XDL on
// macOS): the settings, the API details and the library (package backend).
func Dir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = filepath.Join(os.TempDir())
	}
	return filepath.Join(base, "XDL")
}

// Path is the settings file.
func Path() string { return filepath.Join(Dir(), "settings.json") }

// Store is the settings file with an in-memory copy. It reloads when
// another process (the CLI, say) changed the file.
type Store struct {
	mu      sync.Mutex
	path    string
	cur     Settings
	modTime time.Time
}

// Open loads the settings file, creating it from defaults (or from the
// Objective-C app's preferences, the first time) when missing.
func Open() *Store {
	return OpenPath(Path())
}

// OpenPath is Open with an explicit file, for tests.
func OpenPath(path string) *Store {
	s := &Store{path: path, cur: Defaults()}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); errors.Is(err, fs.ErrNotExist) {
		if migrated, ok := migrateFromUserDefaults(); ok {
			s.cur = migrated.Normalize()
		}
		_ = s.saveLocked()
	}
	return s
}

func (s *Store) loadLocked() error {
	info, err := os.Stat(s.path)
	if err != nil {
		return err
	}
	if info.ModTime().Equal(s.modTime) {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	next := Defaults()
	if err := json.Unmarshal(data, &next); err != nil {
		return fmt.Errorf("settings %s: %w", s.path, err)
	}
	s.cur = next.Normalize()
	s.modTime = info.ModTime()
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	if info, err := os.Stat(s.path); err == nil {
		s.modTime = info.ModTime()
	}
	return nil
}

// Get returns the current settings, reloading the file if it changed.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()
	return s.cur
}

// Set replaces the settings and writes the file.
func (s *Store) Set(next Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = next.Normalize()
	return s.saveLocked()
}

// Update applies fn to the current settings and saves the result.
func (s *Store) Update(fn func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadLocked()
	next := s.cur
	fn(&next)
	s.cur = next.Normalize()
	return s.cur, s.saveLocked()
}

// Keys lists the JSON keys of Settings, for the CLI.
func Keys() []string {
	data, _ := json.Marshal(Defaults())
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SetKey sets one setting from its string form, by JSON key.
func SetKey(s *Settings, key, value string) error {
	data, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	old, ok := m[key]
	if !ok {
		return fmt.Errorf("unknown setting %q (known: %s)", key, strings.Join(Keys(), ", "))
	}
	switch old.(type) {
	case bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s: expected true or false", key)
		}
		m[key] = b
	case float64:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s: expected an integer", key)
		}
		m[key] = n
	default:
		m[key] = value
	}
	data, _ = json.Marshal(m)
	return json.Unmarshal(data, s)
}

// migrateFromUserDefaults reads the Objective-C app's NSUserDefaults
// (domain me.missuo.XDL), so upgrading keeps the user's preferences.
func migrateFromUserDefaults() (Settings, bool) {
	if runtime.GOOS != "darwin" {
		return Settings{}, false
	}
	export := exec.Command("/usr/bin/defaults", "export", "me.missuo.XDL", "-")
	plist, err := export.Output()
	if err != nil || len(plist) == 0 {
		return Settings{}, false
	}
	conv := exec.Command("/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	conv.Stdin = bytes.NewReader(plist)
	out, err := conv.Output()
	if err != nil {
		return Settings{}, false
	}
	var raw map[string]any
	if json.Unmarshal(out, &raw) != nil {
		return Settings{}, false
	}
	s := Defaults()
	found := false
	str := func(key string, dst *string) {
		if v, ok := raw[key].(string); ok {
			*dst, found = v, true
		}
	}
	boolean := func(key string, dst *bool) {
		switch v := raw[key].(type) {
		case bool:
			*dst, found = v, true
		case float64:
			*dst, found = v != 0, true
		}
	}
	integer := func(key string, dst *int) {
		if v, ok := raw[key].(float64); ok {
			*dst, found = int(v), true
		}
	}
	str("downloadPath", &s.DownloadPath)
	integer("concurrentDownloads", &s.ConcurrentDownloads)
	integer("retryAttempts", &s.RetryAttempts)
	boolean("skipExistingFiles", &s.SkipExisting)
	boolean("deleteIncompleteFiles", &s.DeleteIncompleteFiles)
	str("proxy", &s.Proxy)
	integer("fetchTimeout", &s.FetchTimeout)
	boolean("includePhotos", &s.IncludePhotos)
	boolean("includeVideos", &s.IncludeVideos)
	boolean("includeGifs", &s.IncludeGifs)
	boolean("includeText", &s.IncludeText)
	boolean("includeRetweets", &s.Retweets)
	str("videoQuality", &s.VideoQuality)
	str("imageSize", &s.ImageSize)
	str("filenameTemplate", &s.FilenameTemplate)
	str("folderTemplate", &s.FolderTemplate)
	boolean("autoConvertGifs", &s.AutoConvertGIFs)
	str("gifQuality", &s.GIFQuality)
	str("gifResolution", &s.GIFResolution)
	return s, found
}
