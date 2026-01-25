package build

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rhettg/graystone/project"
)

// CacheDir returns the graystone cache directory (~/.cache/graystone).
func CacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "graystone"), nil
}

// BaseImageCached returns true if the base image for the given URL exists in cache.
func BaseImageCached(url string) bool {
	cacheDir, err := CacheDir()
	if err != nil {
		return false
	}

	// Use URL basename as cache filename
	basename := filepath.Base(url)
	cachePath := filepath.Join(cacheDir, "images", basename)

	_, err = os.Stat(cachePath)
	return err == nil
}

// PrintPlan prints the build plan for dry-run output.
func PrintPlan(w io.Writer, p *project.Project) error {
	fmt.Fprintf(w, "Build Plan for: %s\n", p.Root)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 17+len(p.Root)))

	// Base image section
	fmt.Fprintf(w, "Base Image:\n")
	fmt.Fprintf(w, "  URL: %s\n", p.Base)
	if BaseImageCached(p.Base) {
		fmt.Fprintf(w, "  Status: cached\n")
	} else {
		fmt.Fprintf(w, "  Status: not cached (would download)\n")
	}
	fmt.Fprintln(w)

	// Layers section
	fmt.Fprintf(w, "Layers (%d):\n", len(p.Layers))
	for i, layer := range p.Layers {
		fmt.Fprintf(w, "  [%d] %s (%s)\n", i+1, layer.Name, layer.ContentHash[:8])

		// List files that would be copied
		if layer.HasFiles {
			files, err := listLayerFiles(layer.Path)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintf(w, "      - Copy files%s -> %s\n", f, f)
			}
		}

		// Note if script would run
		if layer.HasScript {
			fmt.Fprintf(w, "      - Run install.sh\n")
		}

		fmt.Fprintln(w)
	}

	// Cloud-init section
	if p.CloudInit != "" {
		fmt.Fprintf(w, "Cloud-Init:\n")
		fmt.Fprintf(w, "  - Inject cloud-init.yaml\n")
		fmt.Fprintln(w)
	}

	// Output section
	cacheDir, err := CacheDir()
	if err != nil {
		cacheDir = "~/.cache/graystone"
	} else {
		// Replace home dir with ~ for display
		home, _ := os.UserHomeDir()
		if home != "" {
			cacheDir = strings.Replace(cacheDir, home, "~", 1)
		}
	}
	fmt.Fprintf(w, "Output: %s/builds/<project-hash>.qcow2\n", cacheDir)

	return nil
}

// listLayerFiles returns sorted list of files in a layer's files/ directory.
// Each path is relative to files/ and starts with /.
func listLayerFiles(layerPath string) ([]string, error) {
	filesPath := filepath.Join(layerPath, "files")
	var files []string

	err := filepath.WalkDir(filesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			relPath, err := filepath.Rel(filesPath, path)
			if err != nil {
				return err
			}
			files = append(files, "/"+relPath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}
