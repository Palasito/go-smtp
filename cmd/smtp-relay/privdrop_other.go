//go:build !linux

package main

// ensureDirOwnership is a no-op on non-Linux platforms.
func ensureDirOwnership(_ []string) {}

// dropPrivileges is a no-op on non-Linux platforms.
func dropPrivileges() error { return nil }
