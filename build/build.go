package build

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"github.com/rhettg/tank/project"
)

// CacheDir returns the tank storage directory (/var/lib/tank).
func CacheDir() (string, error) {
	return "/var/lib/tank", nil
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
		fmt.Fprintf(progress, "  %s Using cached image\n", symbolDot)
		return destPath, nil
	}

	// Create cache directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	fmt.Fprintf(progress, "  %s %s\n", symbolDot, mutedStyle.Render(url))

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

	fmt.Fprintf(progress, "  %s Saved to %s\n", symbolSuccess, mutedStyle.Render(destPath))
	return destPath, nil
}

var (
	stylePrimary   = lipgloss.Color("#7C3AED")
	styleSecondary = lipgloss.Color("#06B6D4")
	styleSuccess   = lipgloss.Color("#10B981")
	styleMuted     = lipgloss.Color("#6B7280")

	boldStyle    = lipgloss.NewStyle().Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(styleMuted)
	successStyle = lipgloss.NewStyle().Foreground(styleSuccess)
	infoStyle    = lipgloss.NewStyle().Foreground(styleSecondary)

	symbolInfo    = infoStyle.Render("→")
	symbolSuccess = successStyle.Render("✓")
	symbolDot     = mutedStyle.Render("•")
)

// copyWithProgress copies from src to dst, writing progress to w.
func copyWithProgress(dst io.Writer, src io.Reader, total int64, w io.Writer) (int64, error) {
	var written int64
	buf := make([]byte, 32*1024) // 32KB buffer

	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	for {
		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
				printProgress(w, prog, written, total)
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
				fmt.Fprintln(w)
				return written, nil
			}
			return written, readErr
		}
	}
}

// printProgress writes the current download progress with a nice bar.
func printProgress(w io.Writer, prog progress.Model, written, total int64) {
	if total > 0 {
		pct := float64(written) / float64(total)
		bar := prog.ViewAs(pct)
		stats := fmt.Sprintf("%s / %s", formatBytes(written), formatBytes(total))
		fmt.Fprintf(w, "\r  %s %s", bar, mutedStyle.Render(stats))
	} else {
		fmt.Fprintf(w, "\r  %s", mutedStyle.Render(formatBytes(written)))
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

// BuildImagePath returns the path where the build image should be stored.
func BuildImagePath(projectHash string) (string, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "builds", projectHash+".qcow2"), nil
}

// BuildImageExists returns true if a build image already exists for this project hash.
func BuildImageExists(projectHash string) bool {
	path, err := BuildImagePath(projectHash)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// CreateBuildImage copies the base image to create a new build image.
// Progress is written to the provided writer.
// Returns the path to the new build image.
func CreateBuildImage(baseImagePath, projectHash string, progress io.Writer) (string, error) {
	destPath, err := BuildImagePath(projectHash)
	if err != nil {
		return "", err
	}

	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		fmt.Fprintf(progress, "  Using existing build image: %s\n", destPath)
		return destPath, nil
	}

	// Create builds directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	fmt.Fprintf(progress, "  Source: %s\n", baseImagePath)

	// Open source file
	src, err := os.Open(baseImagePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Get source size for progress
	srcInfo, err := src.Stat()
	if err != nil {
		return "", err
	}

	// Write to temp file, then rename (atomic)
	tmpPath := destPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}

	// Copy with progress tracking
	_, err = copyWithProgress(dst, src, srcInfo.Size(), progress)
	dst.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	// Rename to final path
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", err
	}

	fmt.Fprintf(progress, "  Created: %s\n", destPath)
	return destPath, nil
}

// ApplyLayers applies project layers to a build image using virt-customize.
// Each layer is applied in a separate virt-customize invocation for clear
// error attribution and progress reporting.
func ApplyLayers(imagePath string, layers []project.Layer, progress io.Writer) error {
        applianceDir, err := EnsureGuestfsAppliance(progress)
        if err != nil {
                return err
        }

        for _, layer := range layers {
                args := []string{"-a", imagePath}

		// Copy files first so scripts can reference them
		if layer.HasFiles {
			filesDir := filepath.Join(layer.Path, "files")
			entries, err := os.ReadDir(filesDir)
			if err != nil {
				return fmt.Errorf("reading files dir for layer %s: %w", layer.Name, err)
			}
			for _, entry := range entries {
				src := filepath.Join(filesDir, entry.Name())
				args = append(args, "--copy-in", src+":/")
			}
		}

		// Run install.sh
		if layer.HasScript {
			args = append(args, "--run", filepath.Join(layer.Path, "install.sh"))
		}

		// Register firstboot.sh
		if layer.HasFirstboot {
			args = append(args, "--firstboot", filepath.Join(layer.Path, "firstboot.sh"))
		}

		// Skip if nothing to do (only -a flag)
		if len(args) <= 2 {
			continue
		}

                fmt.Fprintf(progress, "  %s Applying %s\n", symbolDot, boldStyle.Render(layer.Name))

                cmd := exec.Command("virt-customize", args...)
                if applianceDir != "" {
                        cmd.Env = append(os.Environ(), "LIBGUESTFS_PATH="+applianceDir)
                }
                cmd.Stdout = progress
                cmd.Stderr = progress

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("applying layer %s: %w", layer.Name, err)
		}
	}
	return nil
}

// Build runs the full build pipeline: download base image, copy to build image,
// apply layers. Returns the path to the final build image.
// If the build image already exists (cached), it returns early.
func Build(p *project.Project, progress io.Writer) (string, error) {
	projectHash := p.Hash()

	// Check cache
	destPath, err := BuildImagePath(projectHash)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(destPath); err == nil {
		fmt.Fprintf(progress, "%s Build cached %s\n", symbolSuccess, mutedStyle.Render(projectHash[:8]))
		return destPath, nil
	}

	// Download base image
	fmt.Fprintf(progress, "%s Downloading base image\n", symbolInfo)
	baseImagePath, err := DownloadBaseImage(p.Base, progress)
	if err != nil {
		return "", fmt.Errorf("downloading base image: %w", err)
	}
	fmt.Println()

	// Create builds directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	// Copy base to temp build image
	fmt.Fprintf(progress, "%s Creating build image %s\n", symbolInfo, mutedStyle.Render(projectHash[:8]))
	tmpPath := destPath + ".tmp"

	src, err := os.Open(baseImagePath)
	if err != nil {
		return "", err
	}
	srcInfo, err := src.Stat()
	if err != nil {
		src.Close()
		return "", err
	}

	dst, err := os.Create(tmpPath)
	if err != nil {
		src.Close()
		return "", err
	}

	_, err = copyWithProgress(dst, src, srcInfo.Size(), progress)
	dst.Close()
	src.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	// Apply layers
	if len(p.Layers) > 0 {
		fmt.Fprintln(progress)
		fmt.Fprintf(progress, "%s Applying layers\n", symbolInfo)
		if err := ApplyLayers(tmpPath, p.Layers, progress); err != nil {
			os.Remove(tmpPath)
			return "", err
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	return destPath, nil
}

// PrintPlan prints the build plan for dry-run output.
func PrintPlan(w io.Writer, p *project.Project) error {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(stylePrimary)
	highlightStyle := lipgloss.NewStyle().Foreground(styleSecondary)
	headerStyle := lipgloss.NewStyle().Bold(true)

	fmt.Fprintf(w, "%s %s\n\n", titleStyle.Render("Build Plan"), mutedStyle.Render(p.Root))

	// Base image section
	fmt.Fprintf(w, "%s\n", headerStyle.Render("Base Image"))
	fmt.Fprintf(w, "  %s %s\n", symbolDot, mutedStyle.Render(p.Base))
	if BaseImageCached(p.Base) {
		fmt.Fprintf(w, "  %s cached\n", symbolSuccess)
	} else {
		fmt.Fprintf(w, "  %s would download\n", symbolInfo)
	}
	fmt.Fprintln(w)

	// Layers section
	fmt.Fprintf(w, "%s\n", headerStyle.Render(fmt.Sprintf("Layers (%d)", len(p.Layers))))
	for _, layer := range p.Layers {
		fmt.Fprintf(w, "  %s %s\n", boldStyle.Render(layer.Name), mutedStyle.Render(layer.ContentHash[:8]))

		// List files that would be copied
		if layer.HasFiles {
			files, err := listLayerFiles(layer.Path)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintf(w, "      %s copy %s\n", symbolDot, mutedStyle.Render(f))
			}
		}

		// Note if script would run
		if layer.HasScript {
			fmt.Fprintf(w, "      %s run %s\n", symbolDot, highlightStyle.Render("install.sh"))
		}

		// Note if firstboot script would run
		if layer.HasFirstboot {
			fmt.Fprintf(w, "      %s run %s (on first boot)\n", symbolDot, highlightStyle.Render("firstboot.sh"))
		}
	}
	fmt.Fprintln(w)

	// Cloud-init section
	if p.CloudInit != "" {
		fmt.Fprintf(w, "%s\n", headerStyle.Render("Cloud-Init"))
		fmt.Fprintf(w, "  %s inject cloud-init.yaml\n", symbolDot)
		fmt.Fprintln(w)
	}

	// Output section
	projectHash := p.Hash()
	fmt.Fprintf(w, "%s\n", headerStyle.Render("Output"))
	fmt.Fprintf(w, "  %s hash %s\n", symbolDot, highlightStyle.Render(projectHash[:8]))

	buildPath, err := BuildImagePath(projectHash)
	if err != nil {
		buildPath = "/var/lib/tank/builds/" + projectHash + ".qcow2"
	}
	fmt.Fprintf(w, "  %s path %s\n", symbolDot, mutedStyle.Render(buildPath))

	if BuildImageExists(projectHash) {
		fmt.Fprintf(w, "  %s cached (skip build)\n", symbolSuccess)
	} else {
		fmt.Fprintf(w, "  %s would build\n", symbolInfo)
	}

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
