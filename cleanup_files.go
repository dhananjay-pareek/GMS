//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	files := []string{
		"a.txt",
		"cleanup-bloat.bat",
		"cleanup-bloat.sh",
		"cleanup-now.bat",
		"force-push-github.bat",
		"force-push-github.sh",
		"rebrand-status.bat",
		"rebrand-complete.sh",
		"git-release.bat",
		"DEPLOY_READY.md",
		"RENDER_IMPLEMENTATION.md",
		"CLEANUP_README.md",
		"REBRANDING_COMPLETE.md",
		"GITHUB_RELEASE_INSTRUCTIONS.md",
		"URL_UPDATE_SUMMARY.md",
		"test-render-changes.sh",
	}

	deleted := 0
	notFound := 0

	for _, file := range files {
		path := filepath.Join("d:", "gmap-new", file)
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				fmt.Printf("Error deleting %s: %v\n", file, err)
			} else {
				fmt.Printf("✓ Deleted: %s\n", file)
				deleted++
			}
		} else {
			fmt.Printf("✗ Not found: %s\n", file)
			notFound++
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Successfully deleted: %d files\n", deleted)
	fmt.Printf("Not found: %d files\n", notFound)
}
