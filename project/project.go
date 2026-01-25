package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layer represents a single layer in the project.
type Layer struct {
	Name        string // "10-common"
	Path        string // Full path to layer directory
	HasScript   bool   // install.sh exists
	HasFiles    bool   // files/ directory exists
	ContentHash string // SHA256 of layer contents
}

// Project represents a graystone project.
type Project struct {
	Root      string  // Project root directory
	Base      string  // Contents of BASE file
	Layers    []Layer // Sorted lexicographically by name
	CloudInit string  // Contents of cloud-init.yaml (if exists)
}

// Load reads a project from the given path.
func Load(path string) (*Project, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	p := &Project{
		Root: absPath,
	}

	// Read BASE file
	baseContent, err := os.ReadFile(filepath.Join(absPath, "BASE"))
	if err != nil {
		return nil, err
	}
	p.Base = strings.TrimSpace(string(baseContent))

	// Read cloud-init.yaml (optional)
	cloudInitPath := filepath.Join(absPath, "cloud-init.yaml")
	if content, err := os.ReadFile(cloudInitPath); err == nil {
		p.CloudInit = string(content)
	}

	// Load layers
	layersDir := filepath.Join(absPath, "layers")
	entries, err := os.ReadDir(layersDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No layers directory is valid
			return p, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		layerPath := filepath.Join(layersDir, entry.Name())
		layer := Layer{
			Name: entry.Name(),
			Path: layerPath,
		}

		// Check for install.sh
		scriptPath := filepath.Join(layerPath, "install.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			layer.HasScript = true
		}

		// Check for files/ directory
		filesPath := filepath.Join(layerPath, "files")
		if info, err := os.Stat(filesPath); err == nil && info.IsDir() {
			layer.HasFiles = true
		}

		// Compute content hash
		layer.ContentHash, err = hashLayer(layerPath)
		if err != nil {
			return nil, err
		}

		p.Layers = append(p.Layers, layer)
	}

	// Sort layers by name
	sort.Slice(p.Layers, func(i, j int) bool {
		return p.Layers[i].Name < p.Layers[j].Name
	})

	return p, nil
}
