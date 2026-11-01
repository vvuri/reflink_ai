// Package uploadlog appends one line per upload attempt to a log file.
package uploadlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Entry describes a single upload attempt.
type Entry struct {
	Time    time.Time // zero = filled with UTC now
	IP      string
	File    string
	Size    int64
	Format  string // sniffed container name, "" when unknown
	Width   int
	Height  int
	HasEXIF bool
	HasGPS  bool
	Denied  string // "" on success, otherwise a short rejection code
}

// Logger appends entries to a file guarded by a mutex.
type Logger struct {
	mu sync.Mutex
	f  *os.File
}

// New opens (creating if needed) the log file for appending.
func New(path string) (*Logger, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f}, nil
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Upload appends one entry to the log file; concurrent calls are safe.
func (l *Logger) Upload(e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	line := formatLine(e)

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return fmt.Errorf("upload log is closed")
	}
	if _, err := l.f.WriteString(line); err != nil {
		return err
	}
	return l.f.Sync()
}

func formatLine(e Entry) string {
	b := make([]byte, 0, 160)
	b = append(b, e.Time.UTC().Format("2006-01-02T15:04:05Z")...)
	b = append(b, " ip="...)
	b = append(b, e.IP...)
	b = append(b, " file="...)
	b = append(b, strconv.Quote(e.File)...)
	b = append(b, " size="...)
	b = strconv.AppendInt(b, e.Size, 10)
	if e.Format != "" {
		b = append(b, " fmt="...)
		b = append(b, e.Format...)
	}
	if e.Width > 0 && e.Height > 0 {
		b = append(b, " "...)
		b = strconv.AppendInt(b, int64(e.Width), 10)
		b = append(b, 'x')
		b = strconv.AppendInt(b, int64(e.Height), 10)
	}
	b = append(b, " exif="...)
	switch {
	case e.HasEXIF:
		b = append(b, "yes"...)
	default:
		b = append(b, "no"...)
	}
	b = append(b, " gps="...)
	switch {
	case e.HasGPS:
		b = append(b, "yes"...)
	default:
		b = append(b, "no"...)
	}
	switch e.Denied {
	case "":
		b = append(b, " status=ok\n"...)
	default:
		b = append(b, " status=denied reason="...)
		b = append(b, e.Denied...)
		b = append(b, '\n')
	}
	return string(b)
}
