package build

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
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

// BaseImagePath returns the cache path for a base image URL.
func BaseImagePath(url string) (string, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}
	basename := filepath.Base(url)
	return filepath.Join(cacheDir, "images", basename), nil
}

// BaseImageCached returns true if the base image for the given URL exists in cache.
func BaseImageCached(url string) bool {
	cachePath, err := BaseImagePath(url)
	if err != nil {
		return false
	}

	_, err = os.Stat(cachePath)
	return err == nil
}

// DownloadBaseImage downloads the base image if not already cached.
// Progress is written to the provided writer.
// Returns the path to the cached image.
func DownloadBaseImage(url string, progress io.Writer) (string, error) {
	destPath, err := BaseImagePath(url)
	if err != nil {
		return "", err
	}

	// Check if already cached
	if _, err := os.Stat(destPath); err == nil {
		fmt.Fprintf(progress, "  Using cached image: %s\n", destPath)
		return destPath, nil
	}

	// Create cache directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	fmt.Fprintf(progress, "  URL: %s\n", url)

	// Download with progress
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	// Write to temp file, then rename (atomic)
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}

	// Copy with progress tracking
	_, err = copyWithProgress(out, resp.Body, resp.ContentLength, progress)
	out.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	// Rename to final path
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", err
	}

	fmt.Fprintf(progress, "  Saved to: %s\n", destPath)
	return destPath, nil
}

// copyWithProgress copies from src to dst, writing progress to w.
func copyWithProgress(dst io.Writer, src io.Reader, total int64, w io.Writer) (int64, error) {
	var written int64
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
				printProgress(w, written, total)
			}
			if writeErr != nil {
				return written, writeErr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				fmt.Fprintln(w) // Final newline after progress
				return written, nil
			}
			return written, readErr
		}
	}
}

// printProgress writes the current download progress.
func printProgress(w io.Writer, written, total int64) {
	if total > 0 {
		pct := float64(written) / float64(total) * 100
		fmt.Fprintf(w, "\r  Progress: %s / %s (%.0f%%)", formatBytes(written), formatBytes(total), pct)
	} else {
		fmt.Fprintf(w, "\r  Progress: %s", formatBytes(written))
	}
}

// formatBytes formats bytes as human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
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
