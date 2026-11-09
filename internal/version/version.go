package version

import "fmt"

// Info holds version and commit information.
type Info struct {
	Version string
	Commit  string
}

// String returns a formatted version string.
func (i Info) String() string {
	return fmt.Sprintf("skill-manager-cli version %s (commit %s)", i.Version, i.Commit)
}
