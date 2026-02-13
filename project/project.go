package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layer represents a single layer in the project.
type Layer struct {
        Name         string   // "10-common"
        Path         string   // Full path to layer directory
        HasScript    bool     // install exists
        HasFiles     bool     // files/ directory exists
        HasFirstboot bool     // firstboot exists
        HasPreboot   bool     // preboot exists (host-side hook)
        ContentHash  string   // SHA256 of layer contents
        Volumes      []Volume // Volume declarations from volumes/ directory
}

// Project represents a tank project.
type Project struct {
	Root      string  // Project root directory
	Base      string  // Contents of BASE file
	Layers    []Layer // Sorted lexicographically by name
	CloudInit string  // Contents of cloud-init.yaml (if exists)
}

// Hash computes a deterministic hash of the entire project configuration.
// This includes the base image URL, all layer hashes, and cloud-init content.
func (p *Project) Hash() string {
	h := sha256.New()

	// Include base image URL
	h.Write([]byte("base:" + p.Base + "\n"))

	// Include all layer hashes (already sorted)
	for _, layer := range p.Layers {
		h.Write([]byte("layer:" + layer.Name + ":" + layer.ContentHash + "\n"))
	}

	// Include cloud-init if present
	if p.CloudInit != "" {
		h.Write([]byte("cloud-init:\n"))
		h.Write([]byte(p.CloudInit))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// BuildStage represents a cached intermediate build state.
type BuildStage struct {
	Hash  string // cumulative hash up to and including this stage
	Layer *Layer // nil for the base stage
}

// BuildChain computes the sequence of build stages for incremental caching.
// Each stage has a cumulative hash covering the base image, root size, and all
// build-relevant layers up to that point. Layers that don't affect the build
// image (preboot-only, no install/files/firstboot) are skipped.
func (p *Project) BuildChain(rootSize string) []BuildStage {
	// Base stage
	h := sha256.New()
	h.Write([]byte("base:" + p.Base + "\n"))
	h.Write([]byte("rootSize:" + rootSize + "\n"))
	baseHash := hex.EncodeToString(h.Sum(nil))

	stages := []BuildStage{{Hash: baseHash, Layer: nil}}

	prevHash := baseHash
	for i := range p.Layers {
		layer := &p.Layers[i]
		if !layer.HasScript && !layer.HasFiles && !layer.HasFirstboot {
			continue
		}
		h := sha256.New()
		h.Write([]byte(prevHash))
		h.Write([]byte("layer:" + layer.Name + ":" + layer.ContentHash + "\n"))
		prevHash = hex.EncodeToString(h.Sum(nil))
		stages = append(stages, BuildStage{Hash: prevHash, Layer: layer})
	}

	return stages
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
		layerPath := filepath.Join(layersDir, entry.Name())

		// Use os.Stat to follow symlinks
		info, err := os.Stat(layerPath)
		if err != nil || !info.IsDir() {
			continue
		}
		layer := Layer{
			Name: entry.Name(),
			Path: layerPath,
		}

                // Check for install
                scriptPath := filepath.Join(layerPath, "install")
                if _, err := os.Stat(scriptPath); err == nil {
                        layer.HasScript = true
                }

                // Check for firstboot
                firstbootPath := filepath.Join(layerPath, "firstboot")
                if _, err := os.Stat(firstbootPath); err == nil {
                        layer.HasFirstboot = true
                }

		// Check for preboot (host-side hook)
		prebootPath := filepath.Join(layerPath, "preboot")
		if _, err := os.Stat(prebootPath); err == nil {
			layer.HasPreboot = true
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

		// Load volume declarations
		layer.Volumes, err = loadLayerVolumes(layerPath, layer.Name)
		if err != nil {
			return nil, fmt.Errorf("loading volumes for layer %s: %w", layer.Name, err)
		}

		p.Layers = append(p.Layers, layer)
	}

	// Sort layers by name
	sort.Slice(p.Layers, func(i, j int) bool {
		return p.Layers[i].Name < p.Layers[j].Name
	})

	return p, nil
}
