package version

import "fmt"

var (
	Version string
	Commit  string
)

func PrintVersion() {
	fmt.Printf("KubeSkoop version: %s, Git sha: %s\n", Version, Commit)
}
