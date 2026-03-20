// Package logfile provides a time-based rotating log writer with automatic
// retention cleanup. When rotation is enabled, each log file is named with a
// timestamp suffix (e.g. relay-2026-03-20T14.log). A background goroutine
// deletes files older than the configured retention period.
package logfile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RotatingWriter is an io.Writer that rotates log files on a configurable
// time boundary and optionally deletes files older than a retention period.
// It is safe for concurrent use.
type RotatingWriter struct {
	basePath      string        // e.g. /logs/relay.log
	dir           string        // directory portion of basePath
	prefix        string        // filename without extension, e.g. "relay"
	ext           string        // extension including dot, e.g. ".log"
	rotateEvery   time.Duration // 0 = no rotation (single file)
	retentionDays int           // 0 = keep forever

	mu      sync.Mutex
	current *os.File
	bucket  string // current time bucket key, e.g. "2026-03-20T14"

	cancel context.CancelFunc
	done   chan struct{}

	// BannerFunc, if set, is called with the newly opened file whenever a
	// rotation occurs. Use it to write a startup/rotation header.
	BannerFunc func(f *os.File)
}

// Options configures the rotating writer.
type Options struct {
	// BasePath is the log file path (e.g. "/logs/relay.log").
	// When rotation is enabled the timestamp is inserted before the extension:
	// /logs/relay-2026-03-20T14.log
	BasePath string

	// RotateEvery is the rotation interval. Use multiples of time.Hour.
	// 0 means no rotation — behaves as a plain append-only file.
	RotateEvery time.Duration

	// RetentionDays is how many days of log files to keep.
	// 0 means keep forever.
	RetentionDays int

	// BannerFunc is called each time a new log file is opened.
	BannerFunc func(f *os.File)
}

// New creates a RotatingWriter and opens the initial log file.
// The caller must call Close when done.
func New(ctx context.Context, opts Options) (*RotatingWriter, error) {
	dir := filepath.Dir(opts.BasePath)
	base := filepath.Base(opts.BasePath)
	ext := filepath.Ext(base)
	prefix := strings.TrimSuffix(base, ext)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	inner, innerCancel := context.WithCancel(ctx)

	rw := &RotatingWriter{
		basePath:      opts.BasePath,
		dir:           dir,
		prefix:        prefix,
		ext:           ext,
		rotateEvery:   opts.RotateEvery,
		retentionDays: opts.RetentionDays,
		cancel:        innerCancel,
		done:          make(chan struct{}),
		BannerFunc:    opts.BannerFunc,
	}

	if err := rw.openFile(time.Now()); err != nil {
		innerCancel()
		return nil, err
	}

	go rw.backgroundLoop(inner)
	return rw, nil
}

// Write implements io.Writer. It rotates the file if the time bucket changed.
func (rw *RotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.rotateEvery > 0 {
		b := rw.timeBucket(time.Now())
		if b != rw.bucket {
			if err := rw.rotateToLocked(b); err != nil {
				// Best-effort: log to old file and continue.
				slog.Warn("Log rotation failed, continuing with current file", "error", err)
			}
		}
	}

	if rw.current == nil {
		return 0, fmt.Errorf("log file not open")
	}
	return rw.current.Write(p)
}

// Close flushes and closes the current file and stops the background goroutine.
func (rw *RotatingWriter) Close() error {
	rw.cancel()
	<-rw.done

	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.current != nil {
		err := rw.current.Close()
		rw.current = nil
		return err
	}
	return nil
}

// timeBucket returns the time bucket key for the given instant.
func (rw *RotatingWriter) timeBucket(t time.Time) string {
	if rw.rotateEvery <= 0 {
		return ""
	}
	// Truncate to the rotation boundary.
	trunc := t.Truncate(rw.rotateEvery)
	if rw.rotateEvery >= 24*time.Hour {
		return trunc.Format("2006-01-02")
	}
	return trunc.Format("2006-01-02T15")
}

// filenameForBucket returns the full path for a given time bucket.
// If rotation is disabled it returns the original base path.
func (rw *RotatingWriter) filenameForBucket(bucket string) string {
	if bucket == "" {
		return rw.basePath
	}
	return filepath.Join(rw.dir, rw.prefix+"-"+bucket+rw.ext)
}

// openFile opens (or creates) the file for the time bucket containing t.
// Caller must hold rw.mu.
func (rw *RotatingWriter) openFile(t time.Time) error {
	bucket := rw.timeBucket(t)
	path := rw.filenameForBucket(bucket)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}

	rw.current = f
	rw.bucket = bucket

	if rw.BannerFunc != nil {
		rw.BannerFunc(f)
	}

	return nil
}

// rotateToLocked closes the current file and opens a new one for the given bucket.
// Caller must hold rw.mu.
func (rw *RotatingWriter) rotateToLocked(bucket string) error {
	if rw.current != nil {
		rw.current.Close()
		rw.current = nil
	}

	path := rw.filenameForBucket(bucket)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open rotated log file %q: %w", path, err)
	}

	rw.current = f
	rw.bucket = bucket

	if rw.BannerFunc != nil {
		rw.BannerFunc(f)
	}

	return nil
}

// backgroundLoop runs the retention cleanup on a regular schedule.
func (rw *RotatingWriter) backgroundLoop(ctx context.Context) {
	defer close(rw.done)

	// Determine tick interval: check once per hour or once per rotation period,
	// whichever is shorter, but at least every hour.
	tick := time.Hour
	if rw.rotateEvery > 0 && rw.rotateEvery < tick {
		tick = rw.rotateEvery
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if rw.retentionDays > 0 {
				rw.cleanup()
			}
		}
	}
}

// cleanup removes log files older than the retention period.
func (rw *RotatingWriter) cleanup() {
	cutoff := time.Now().Add(-time.Duration(rw.retentionDays) * 24 * time.Hour)

	entries, err := os.ReadDir(rw.dir)
	if err != nil {
		slog.Warn("Log cleanup: failed to read log directory", "dir", rw.dir, "error", err)
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only consider files that match our prefix and extension pattern.
		if !strings.HasPrefix(name, rw.prefix) || !strings.HasSuffix(name, rw.ext) {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(rw.dir, name)
			// Never delete the currently active file.
			rw.mu.Lock()
			currentPath := ""
			if rw.current != nil {
				currentPath = rw.filenameForBucket(rw.bucket)
			}
			rw.mu.Unlock()

			if path == currentPath {
				continue
			}

			if err := os.Remove(path); err != nil {
				slog.Warn("Log cleanup: failed to delete old log file",
					"path", path, "error", err)
			} else {
				slog.Info("Log cleanup: deleted old log file",
					"path", path, "age", time.Since(info.ModTime()).Round(time.Hour))
			}
		}
	}
}
