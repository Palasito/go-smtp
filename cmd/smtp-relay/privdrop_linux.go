//go:build linux

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

const (
	dropUID = 65534
	dropGID = 65534
)

// ensureDirOwnership creates each directory (if needed) and chowns it to the
// target UID/GID. Only effective when the process is running as root.
func ensureDirOwnership(paths []string) {
	if os.Getuid() != 0 {
		return
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Warn("Could not create directory", "path", dir, "error", err)
			continue
		}
		if err := os.Chown(dir, dropUID, dropGID); err != nil {
			slog.Warn("Could not chown directory", "path", dir, "error", err)
		}
	}
}

// dropPrivileges switches the process from root to UID/GID 65534 (nobody).
// If the process is already non-root, this is a no-op.
func dropPrivileges() error {
	if os.Getuid() != 0 {
		return nil
	}
	if err := syscall.Setgroups([]int{dropGID}); err != nil {
		return fmt.Errorf("setgroups: %w", err)
	}
	if err := syscall.Setgid(dropGID); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err := syscall.Setuid(dropUID); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}
	slog.Info("Dropped privileges", "uid", dropUID, "gid", dropGID)
	return nil
}
