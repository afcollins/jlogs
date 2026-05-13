// Package version holds version information set at build time via ldflags.
package version

var (
	// Tag is the git tag set at build time (e.g., "v1.0.0").
	Tag = "dev"
	// Hash is the git commit hash set at build time (e.g., "a1b2c3d").
	Hash = "unknown"
)
