package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// hashLayer computes a deterministic SHA256 hash of a layer's contents.
// It includes:
// - install.sh content (if exists)
// - For each file in files/ (sorted by path): relative path, file mode, content
func hashLayer(layerPath string) (string, error) {
	h := sha256.New()

	// Hash install.sh if it exists
	scriptPath := filepath.Join(layerPath, "install.sh")
	if content, err := os.ReadFile(scriptPath); err == nil {
		h.Write([]byte("script:install.sh\n"))
		h.Write(content)
	}

	// Hash files/ directory contents
	filesPath := filepath.Join(layerPath, "files")
	if _, err := os.Stat(filesPath); err == nil {
		if err := hashFilesDir(h, filesPath); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashFilesDir walks the files/ directory and hashes all files deterministically.
func hashFilesDir(h io.Writer, filesPath string) error {
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
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Sort for deterministic ordering
	sort.Strings(files)

	for _, relPath := range files {
		fullPath := filepath.Join(filesPath, relPath)

		info, err := os.Stat(fullPath)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}

		// Write path, mode, and content to hash
		fmt.Fprintf(h, "file:%s\n", relPath)
		fmt.Fprintf(h, "mode:%o\n", info.Mode().Perm())
		h.Write(content)
	}

	return nil
}
