// Package version holds the build-time version string, injected via -ldflags.
package version

// Version is overridden at build time with -ldflags "-X mycnfm/internal/version.Version=...".
var Version = "dev"
