package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The download engine: every file goes through <file>.part, which a retry —
// or a later run, after a stop — continues with an HTTP Range request
// instead of starting over. Files of SegmentThreshold and more download in
// SegmentCount ranges at once, each in its own <file>.partN, continued the
// same way, and joined when all are done. No request times out as a whole:
// a transfer fails only when no byte arrives for StallTimeout, so a large
// video on a slow line finishes.

// SegmentCount is how many ranges a large file downloads in at once.
const SegmentCount = 4

// Variables for tests.
var (
	SegmentThreshold int64 = 50 << 20
	FirstChunk       int64 = 2 << 20
	StallTimeout           = 60 * time.Second
)

func partPath(outputPath string) string { return outputPath + ".part" }

func segmentPath(outputPath string, i int) string {
	return outputPath + ".part" + strconv.Itoa(i)
}

// removeParts deletes what an unfinished download left.
func removeParts(outputPath string) {
	_ = os.Remove(partPath(outputPath))
	for i := 0; i < SegmentCount; i++ {
		_ = os.Remove(segmentPath(outputPath, i))
	}
}

func hasSegments(outputPath string) bool {
	for i := 0; i < SegmentCount; i++ {
		if _, err := os.Stat(segmentPath(outputPath, i)); err == nil {
			return true
		}
	}
	return false
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// streaming returns client without its overall timeout: transfers are
// bounded by StallTimeout instead.
func streaming(client *http.Client) *http.Client {
	c := *client
	c.Timeout = 0
	return &c
}

// contentRange parses "bytes 100-199/1000" (or "bytes */1000") into its
// start and the total, -1 when unknown.
func contentRange(h string) (start, total int64, ok bool) {
	h = strings.TrimSpace(strings.TrimPrefix(h, "bytes "))
	rng, tot, found := strings.Cut(h, "/")
	if !found {
		return 0, 0, false
	}
	total = -1
	if tot != "*" {
		t, err := strconv.ParseInt(tot, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		total = t
	}
	if rng == "*" {
		return -1, total, true
	}
	s, _, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return start, total, true
}

// get requests url from byte from on, or all of it from -1.
func get(ctx context.Context, client *http.Client, url string, from, to int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case from >= 0 && to >= 0:
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	case from >= 0:
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", from))
	}
	return client.Do(req)
}

// copyStalling copies src to dst, failing when no byte arrives for
// StallTimeout. cancel stops the request that src reads.
func copyStalling(dst io.Writer, src io.Reader, cancel context.CancelFunc) (int64, error) {
	var stalled bool
	var mu sync.Mutex
	timer := time.AfterFunc(StallTimeout, func() {
		mu.Lock()
		stalled = true
		mu.Unlock()
		cancel()
	})
	defer timer.Stop()
	buf := make([]byte, 256<<10)
	var n int64
	for {
		r, err := src.Read(buf)
		if r > 0 {
			timer.Reset(StallTimeout)
			w, werr := dst.Write(buf[:r])
			n += int64(w)
			if werr != nil {
				return n, werr
			}
		}
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			mu.Lock()
			s := stalled
			mu.Unlock()
			if s {
				return n, fmt.Errorf("no data for %s", StallTimeout)
			}
			return n, err
		}
	}
}

// fetchToPart downloads url into outputPath.part, continuing the part a
// previous try left, or into segments for a large file. It returns once
// the part holds the whole file.
//
// A fresh download asks for its first FirstChunk bytes alone: small files
// (most photos) come whole in that one request, and a larger one says its
// size without the server pushing bytes nobody reads. No request but one
// that reads to the end is open-ended, so a switch to segments, a stop or
// a failure wastes nothing.
func fetchToPart(ctx context.Context, client *http.Client, url, outputPath string) error {
	client = streaming(client)
	if hasSegments(outputPath) {
		total, err := probeSize(ctx, client, url)
		if err != nil {
			return err
		}
		if total > 0 {
			return fetchSegments(ctx, client, url, outputPath, total)
		}
		removeParts(outputPath) // the server stopped telling the size
	}

	part := partPath(outputPath)
	offset := fileSize(part)
	fresh := offset == 0
	end := int64(-1)
	if fresh {
		end = FirstChunk - 1
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := get(reqCtx, client, url, offset, end)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var file *os.File
	expect := int64(-1) // bytes the body should hold
	total := int64(-1)
	switch resp.StatusCode {
	case http.StatusPartialContent:
		var start int64
		var ok bool
		start, total, ok = contentRange(resp.Header.Get("Content-Range"))
		if !ok || start != offset {
			return fmt.Errorf("unexpected Content-Range %q", resp.Header.Get("Content-Range"))
		}
		expect = resp.ContentLength
		file, err = os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	case http.StatusOK:
		// No ranges: the whole file.
		expect = resp.ContentLength
		file, err = os.Create(part)
	case http.StatusRequestedRangeNotSatisfiable:
		_, t, ok := contentRange(resp.Header.Get("Content-Range"))
		if ok && t == offset && offset > 0 {
			return nil // the part was already whole
		}
		_ = os.Remove(part)
		return fmt.Errorf("the partial download no longer matches the file; starting over")
	default:
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	if err != nil {
		return err
	}
	n, err := copyStalling(file, resp.Body, cancel)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if expect >= 0 && n != expect {
		return fmt.Errorf("the connection closed after %d of %d bytes", n, expect)
	}
	if resp.StatusCode == http.StatusOK || total < 0 || offset+n >= total {
		return nil // whole
	}
	resp.Body.Close()
	cancel()
	if fresh && total >= SegmentThreshold {
		// The first chunk starts the first segment.
		if err := os.Rename(part, segmentPath(outputPath, 0)); err != nil {
			return err
		}
		return fetchSegments(ctx, client, url, outputPath, total)
	}
	return fetchToPart(ctx, client, url, outputPath) // the rest, in one stream
}

// probeSize asks for the first byte to learn the file's size, or 0 when the
// server will not say or serve ranges.
func probeSize(ctx context.Context, client *http.Client, url string) (int64, error) {
	resp, err := get(ctx, client, url, 0, 0)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, nil
	}
	_, total, ok := contentRange(resp.Header.Get("Content-Range"))
	if !ok || total <= 0 {
		return 0, nil
	}
	return total, nil
}

// fetchSegments downloads total bytes in SegmentCount ranges at once, each
// into its .partN, continuing those a previous try left, then joins them
// into the .part. A segment that fails does not stop the others: what they
// fetch is kept, and the retry asks for the rest of the failed one alone.
func fetchSegments(ctx context.Context, client *http.Client, url, outputPath string, total int64) error {
	size := (total + SegmentCount - 1) / SegmentCount
	errs := make([]error, SegmentCount)
	var wg sync.WaitGroup
	for i := 0; i < SegmentCount; i++ {
		start := int64(i) * size
		end := min(start+size, total) - 1
		if start > end {
			continue
		}
		wg.Go(func() {
			errs[i] = fetchSegment(ctx, client, url, segmentPath(outputPath, i), start, end)
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	part, err := os.Create(partPath(outputPath))
	if err != nil {
		return err
	}
	for i := 0; i < SegmentCount; i++ {
		seg, err := os.Open(segmentPath(outputPath, i))
		if errors.Is(err, os.ErrNotExist) {
			continue // a range past the end of a small file
		}
		if err != nil {
			part.Close()
			return err
		}
		_, err = io.Copy(part, seg)
		seg.Close()
		if err != nil {
			part.Close()
			return err
		}
	}
	if err := part.Close(); err != nil {
		return err
	}
	if got := fileSize(partPath(outputPath)); got != total {
		_ = os.Remove(partPath(outputPath))
		return fmt.Errorf("joined %d of %d bytes", got, total)
	}
	for i := 0; i < SegmentCount; i++ {
		_ = os.Remove(segmentPath(outputPath, i))
	}
	return nil
}

func fetchSegment(ctx context.Context, client *http.Client, url, path string, start, end int64) error {
	want := end - start + 1
	have := fileSize(path)
	if have == want {
		return nil
	}
	if have > want {
		// A first chunk longer than the segment: its prefix is the segment.
		if err := os.Truncate(path, want); err != nil {
			return err
		}
		return nil
	}
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	resp, err := get(reqCtx, client, url, start+have, end)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range request answered %s", resp.Status)
	}
	if s, _, ok := contentRange(resp.Header.Get("Content-Range")); !ok || s != start+have {
		return fmt.Errorf("unexpected Content-Range %q", resp.Header.Get("Content-Range"))
	}
	body := resp.Body
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	n, err := copyStalling(file, body, cancel)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if have+n != want {
		return fmt.Errorf("the connection closed after %d of %d bytes", have+n, want)
	}
	return nil
}

// ensureDir makes the directory a file goes in.
func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
