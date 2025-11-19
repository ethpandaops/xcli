// Package version provides build version information for xcli.
// These variables are set at build time via ldflags.
package version

var (
	// Version is the semantic version of the application.
	Version = "dev"

	// Commit is the git commit hash.
	Commit = "none"

	// Date is the build date.
	Date = "unknown"
)

// GetVersion returns the current version string.
func GetVersion() string {
	return Version
}

// GetFullVersion returns a formatted version string with all build info.
func GetFullVersion() string {
	return Version + " (commit: " + Commit + ", built: " + Date + ")"
}
