// Package version holds build information injected by the linker.
package version

// Set via -ldflags "-X github.com/inphaseye172/playanything/internal/version.Version=v1.2.3".
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// String formats the version for `playanything version`.
func String() string {
	s := Version
	if Commit != "" {
		s += " (" + Commit
		if Date != "" {
			s += ", " + Date
		}
		s += ")"
	}
	return s
}
