// Package version exposes build information injected at link time.
package version

// Set via -ldflags "-X github.com/fs0ciety7000/aether-media/internal/version.Version=..."
var (
	Version = "dev"
	Commit  = "unknown"
)
