package uploadlog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reflink/internal/uploadlog"
)

func TestLinesOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "logs", "uploads.log")
	l, err := uploadlog.New(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := l.Upload(uploadlog.Entry{
		Time:    time.Unix(0, 0).UTC(),
		IP:      "203.0.113.7",
		File:    `D:\pics\IMG "1".jpg`,
		Size:    3821945,
		Format:  "JPEG",
		Width:   4032,
		Height:  3024,
		HasEXIF: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := l.Upload(uploadlog.Entry{
		IP:     "198.51.100.2",
		File:   "notes.txt",
		Size:   42,
		Denied: "unsupported-format",
	}); err != nil {
		t.Fatal(err)
	}

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	// After a Close the logger must refuse to write instead of crashing.
	if err := l.Upload(uploadlog.Entry{IP: "1.1.1.1"}); err == nil {
		t.Fatal("close must be final")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(rows) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(rows), data)
	}

	first := " ip=203.0.113.7 " +
		`file="D:\\pics\\IMG \"1\".jpg"` +
		" size=3821945 fmt=JPEG 4032x3024 exif=yes gps=no status=ok"
	if !strings.HasSuffix(rows[0], first) {
		t.Fatalf("line 1 mismatch:\n got %q\nwant suffix %q", rows[0], first)
	}

	second := " ip=198.51.100.2 file=\"notes.txt\" size=42 exif=no gps=no status=denied reason=unsupported-format"
	if !strings.HasSuffix(rows[1], second) {
		t.Fatalf("line 2 mismatch:\n got %q\nwant suffix %q", rows[1], second)
	}
}

func TestTimestampFilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uploads.log")
	l, err := uploadlog.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Upload(uploadlog.Entry{IP: "127.0.0.1", File: "a.png"}); err != nil {
		t.Fatal(err)
	}
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Format("2006-01-02T15:")
	if strings.HasPrefix(string(data), hour) {
		return // timestamp auto-filled — good
	}
	t.Fatalf("timestamp not auto-filled: %q", data)
}
