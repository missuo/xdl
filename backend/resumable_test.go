package backend

import (
	"bytes"
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// media serves content with Range support, and records what it was asked.
type media struct {
	content []byte
	mu      sync.Mutex
	ranges  []string
	served  atomic.Int64
	// cut, when set, aborts the first n responses after that many bytes.
	cut, cuts int
	noRanges  bool
	hang      bool
}

func (m *media) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.ranges = append(m.ranges, r.Header.Get("Range"))
	cut := m.cuts > 0 && m.cut > 0
	if cut {
		m.cuts--
	}
	m.mu.Unlock()
	if m.hang {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		return
	}
	if m.noRanges {
		r.Header.Del("Range")
	}
	cw := &countingWriter{ResponseWriter: w, m: m}
	if cut {
		cw.limit = m.cut
	}
	http.ServeContent(cw, r, "", time.Time{}, bytes.NewReader(m.content))
}

type countingWriter struct {
	http.ResponseWriter
	m     *media
	limit int
	n     int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	if c.limit > 0 && c.n+len(p) > c.limit {
		p = p[:c.limit-c.n]
		c.ResponseWriter.Write(p)
		c.m.served.Add(int64(len(p)))
		c.ResponseWriter.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // the connection drops mid-file
	}
	c.n += len(p)
	c.m.served.Add(int64(len(p)))
	return c.ResponseWriter.Write(p)
}

func randomBytes(t *testing.T, n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func download(t *testing.T, m *media, retries int) (string, error) {
	t.Helper()
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	out := filepath.Join(t.TempDir(), "sub", "video.mp4")
	err := downloadFileWithRetry(context.Background(), srv.Client(), srv.URL+"/v.mp4", out,
		DownloadOptions{RetryAttempts: retries, SkipExistingFiles: true, DeleteIncompleteFiles: true})
	return out, err
}

func checkFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content differs: %d bytes, want %d", len(got), len(want))
	}
	matches, _ := filepath.Glob(path + ".part*")
	if len(matches) > 0 {
		t.Errorf("left %v behind", matches)
	}
}

func TestDownloadWhole(t *testing.T) {
	m := &media{content: randomBytes(t, 300<<10)}
	out, err := download(t, m, 1)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
}

func TestResumeAfterDrop(t *testing.T) {
	m := &media{content: randomBytes(t, 1<<20), cut: 400 << 10, cuts: 1}
	out, err := download(t, m, 2)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
	if len(m.ranges) != 2 || m.ranges[1] != "bytes=409600-" {
		t.Errorf("ranges %q: the retry should continue at 409600", m.ranges)
	}
	if got := m.served.Load(); got != int64(len(m.content)) {
		t.Errorf("served %d bytes for a %d-byte file", got, len(m.content))
	}
}

func TestServerWithoutRanges(t *testing.T) {
	m := &media{content: randomBytes(t, 1<<20), cut: 400 << 10, cuts: 1, noRanges: true}
	out, err := download(t, m, 2)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
}

// smallSegments makes files of 1 MB and more download in segments.
func smallSegments(t *testing.T) {
	threshold, chunk := SegmentThreshold, FirstChunk
	SegmentThreshold, FirstChunk = 1<<20, 128<<10
	t.Cleanup(func() { SegmentThreshold, FirstChunk = threshold, chunk })
}

func TestSegments(t *testing.T) {
	smallSegments(t)
	m := &media{content: randomBytes(t, 3<<20+12345)}
	out, err := download(t, m, 0)
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
	segs := 0
	for _, r := range m.ranges {
		if strings.HasPrefix(r, "bytes=") && !strings.HasSuffix(r, "-") {
			segs++
		}
	}
	if segs != SegmentCount+1 || m.served.Load() != int64(len(m.content)) {
		t.Errorf("ranges %q, %d bytes served: want the first chunk, %d segments and every byte once",
			m.ranges, m.served.Load(), SegmentCount)
	}
}

func TestSegmentResume(t *testing.T) {
	smallSegments(t)
	// The probe passes; the first segment request drops after 100 KB.
	m := &media{content: randomBytes(t, 4<<20)}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	out := filepath.Join(t.TempDir(), "v.mp4")
	m.cut, m.cuts = 100<<10, 0
	first := true
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		if first && r.Header.Get("Range") != "bytes=0-131071" {
			first, m.cuts = false, 1
		}
		m.mu.Unlock()
		m.ServeHTTP(w, r)
	})
	srv.Config.Handler = wrapped
	err := downloadFileWithRetry(context.Background(), srv.Client(), srv.URL+"/v.mp4", out,
		DownloadOptions{RetryAttempts: 2, SkipExistingFiles: true, DeleteIncompleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
	// Every byte once (and the one the retry's size probe asks for): the
	// other segments finish, and the retry asks for the rest of the
	// dropped one alone.
	if got := m.served.Load(); got != int64(len(m.content))+1 {
		t.Errorf("served %d bytes for a %d-byte file (ranges %q)", got, len(m.content), m.ranges)
	}
}

func TestStall(t *testing.T) {
	old := StallTimeout
	StallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { StallTimeout = old })
	m := &media{hang: true}
	start := time.Now()
	_, err := download(t, m, 0)
	if err == nil || !strings.Contains(err.Error(), "no data") {
		t.Fatalf("err %v, want a stall", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("took %s", time.Since(start))
	}
}

func TestStopKeepsThePart(t *testing.T) {
	m := &media{content: randomBytes(t, 1<<20), cut: 300 << 10, cuts: 1}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	out := filepath.Join(t.TempDir(), "v.mp4")
	// One try, no retry, incomplete files kept: the part stays.
	err := downloadFileWithRetry(context.Background(), srv.Client(), srv.URL, out, DownloadOptions{RetryAttempts: 0})
	if err == nil {
		t.Fatal("the dropped download succeeded")
	}
	if n := fileSize(partPath(out)); n != 300<<10 {
		t.Fatalf("part holds %d bytes", n)
	}
	// The next run continues it.
	if err := downloadFileWithRetry(context.Background(), srv.Client(), srv.URL, out, DownloadOptions{}); err != nil {
		t.Fatal(err)
	}
	checkFile(t, out, m.content)
}
