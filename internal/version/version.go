// Package version holds build-time metadata injected via -ldflags.
package version

// These variables are set at build time using -ldflags:
//
//	go build -ldflags="-X github.com/Palasito/go-smtp/internal/version.Version=v1.5.0
//	  -X github.com/Palasito/go-smtp/internal/version.Commit=abc1234
//	  -X github.com/Palasito/go-smtp/internal/version.BuildDate=2025-01-01T00:00:00Z"
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
