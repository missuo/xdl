package gui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/webp"

	"github.com/missuo/xdl/backend"
)

// The profile page's pictures load as they come into view: thumbnails
// eight at a time and the viewer's picture in slots of its own, so it never
// waits behind a grid of thumbnails. Decoded pictures stay in memory up to
// imageMemory, the least recently shown going first; thumbnails, avatars
// and banners stay on disk as well, so a profile opened again shows at
// once. A picture XDL downloaded shows from its file.

type imageKind int

const (
	imgThumb imageKind = iota
	imgPreview
	imgAvatar
	imgBanner
)

const (
	imageMemory  = 256 << 20
	imageRetry   = 30 * time.Second
	imageStale   = 3 * time.Second // a thumbnail not asked for since is no longer on screen
	thumbLimit   = 8 << 20
	previewLimit = 40 << 20
)

type imageEntry struct {
	bm       *ui.Bitmap
	bytes    int
	used     uint64
	pending  bool
	shown    bool
	released bool
	failed   time.Time
	wanted   atomic.Int64 // when the view last asked, for the loader's goroutine
}

type imageLoader struct {
	win   *mygo.Window
	proxy func() string

	// Main thread only.
	entries map[string]*imageEntry
	clock   uint64
	bytes   int
	keep    string // the viewer's picture, never evicted

	thumbSlots   chan struct{}
	previewSlots chan struct{}

	clientMu    sync.Mutex
	client      *http.Client
	clientProxy string
}

func newImageLoader(win *mygo.Window, proxy func() string) *imageLoader {
	return &imageLoader{
		win:          win,
		proxy:        proxy,
		entries:      map[string]*imageEntry{},
		thumbSlots:   make(chan struct{}, 8),
		previewSlots: make(chan struct{}, 2),
	}
}

// get returns the picture at url (or the file at local, when there is
// one), or nil while it loads. Main thread only.
func (l *imageLoader) get(url, local string, kind imageKind) *ui.Bitmap {
	if url == "" && local == "" {
		return nil
	}
	key := fmt.Sprintf("%d|%s|%s", kind, url, local)
	e := l.entries[key]
	if e == nil {
		e = &imageEntry{}
		l.entries[key] = e
	}
	l.clock++
	e.used = l.clock
	e.wanted.Store(time.Now().UnixNano())
	if kind == imgPreview {
		l.keep = key
	}
	if e.bm != nil {
		// Once a picture has shown, the GPU has it: drop the CPU's copy,
		// which DecodeBitmap can make again from the data.
		if e.shown && !e.released {
			e.bm.ReleasePixels()
			e.released = true
		}
		e.shown = true
		return e.bm
	}
	if !e.pending && time.Since(e.failed) > imageRetry {
		e.pending = true
		go l.load(key, url, local, kind, e)
	}
	return nil
}

func (l *imageLoader) load(key, url, local string, kind imageKind, e *imageEntry) {
	slots := l.thumbSlots
	if kind == imgPreview {
		slots = l.previewSlots
	}
	slots <- struct{}{}
	defer func() { <-slots }()
	if kind == imgThumb && time.Since(time.Unix(0, e.wanted.Load())) > imageStale {
		l.win.Update(func() { e.pending = false }) // scrolled past before its turn
		return
	}
	data, err := l.read(url, local, kind)
	var bm *ui.Bitmap
	size := 0
	if err == nil {
		bm, size, err = decodePicture(data)
	}
	l.win.Update(func() {
		e.pending = false
		if err != nil {
			e.failed = time.Now()
			return
		}
		e.bm, e.bytes, e.shown, e.released = bm, size, false, false
		l.bytes += size
		l.evict()
	})
}

// evict drops the least recently shown pictures beyond imageMemory.
func (l *imageLoader) evict() {
	for l.bytes > imageMemory {
		oldest, key := uint64(1<<63), ""
		for k, e := range l.entries {
			if e.bm != nil && k != l.keep && e.used < oldest {
				oldest, key = e.used, k
			}
		}
		if key == "" {
			return
		}
		l.bytes -= l.entries[key].bytes
		delete(l.entries, key)
	}
}

func decodePicture(data []byte) (*ui.Bitmap, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, 0, err
	}
	// Its pixels, and the halved copies a smaller drawing makes.
	return bm, cfg.Width * cfg.Height * 4 * 4 / 3, nil
}

func (l *imageLoader) read(url, local string, kind imageKind) ([]byte, error) {
	if local != "" {
		if info, err := os.Stat(local); err == nil && info.Size() <= previewLimit {
			return os.ReadFile(local)
		}
		if url == "" {
			return nil, errors.New("the file is gone")
		}
	}
	cached := ""
	if kind != imgPreview {
		if dir, err := os.UserCacheDir(); err == nil {
			sum := sha256.Sum256([]byte(url))
			cached = filepath.Join(dir, "XDL", "images", hex.EncodeToString(sum[:16]))
			if data, err := os.ReadFile(cached); err == nil {
				return data, nil
			}
		}
	}
	limit := int64(thumbLimit)
	if kind == imgPreview {
		limit = previewLimit
	}
	data, err := l.download(url, limit)
	if err == nil && cached != "" {
		writeCache(cached, data)
	}
	return data, err
}

func (l *imageLoader) download(url string, limit int64) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://x.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	resp, err := l.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("the picture is too large")
	}
	return data, nil
}

func (l *imageLoader) httpClient() *http.Client {
	proxy := l.proxy()
	l.clientMu.Lock()
	defer l.clientMu.Unlock()
	if l.client == nil || l.clientProxy != proxy {
		c, err := backend.CreateHTTPClient(proxy, 60*time.Second)
		if err != nil {
			c = &http.Client{Timeout: 60 * time.Second}
		}
		l.client, l.clientProxy = c, proxy
	}
	return l.client
}

func writeCache(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
