// Package logbuf keeps the process's recent log lines in memory, so the GUI
// can show them, while still writing them to stderr.
package logbuf

import (
	"bytes"
	"io"
	"log"
	"os"
	"sync"
)

const maxLines = 2000

// Buffer is an io.Writer that tees log output and remembers recent lines.
type Buffer struct {
	mu      sync.Mutex
	out     io.Writer
	partial []byte
	lines   []string
	seq     uint64
}

// Default is the buffer Install connects to the standard logger.
var Default = &Buffer{out: os.Stderr}

// Install routes the standard logger through Default.
func Install(prefix string) {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix(prefix)
	log.SetOutput(Default)
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.out != nil {
		_, _ = b.out.Write(p)
	}
	b.partial = append(b.partial, p...)
	for {
		i := bytes.IndexByte(b.partial, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, string(b.partial[:i]))
		b.partial = b.partial[i+1:]
		b.seq++
	}
	if over := len(b.lines) - maxLines; over > 0 {
		b.lines = append([]string(nil), b.lines[over:]...)
	}
	return len(p), nil
}

// SetOutput changes where lines are copied to; nil copies them nowhere.
func (b *Buffer) SetOutput(w io.Writer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.out = w
}

// Lines returns a copy of the remembered lines and a sequence number that
// changes whenever a line is added.
func (b *Buffer) Lines() ([]string, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.lines...), b.seq
}

// Seq returns the sequence number alone, cheaply.
func (b *Buffer) Seq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// Clear forgets the remembered lines.
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = nil
	b.seq++
}
