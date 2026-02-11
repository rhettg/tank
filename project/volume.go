package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// VolumeKind identifies the type of volume declaration.
type VolumeKind int

const (
	VolumeBlock   VolumeKind = iota // has size: → qcow2
	VolumeRoot                      // file named "root" → root disk override
	VolumeNetwork                   // has source: → network mount
)

// Volume represents a volume declaration from a layer's volumes/ directory.
type Volume struct {
	Name    string     // filename (e.g. "pgdata", "root", "shared")
	Kind    VolumeKind // block, root, or network
	Mount   string     // mount point inside VM
	Size    string     // e.g. "20G"
	Format  string     // filesystem format (default "ext4")
	Owner   string     // chown mount point (e.g. "postgres:postgres")
	Source  string     // network mount source (e.g. "192.168.1.10:/export/data")
	Type    string     // network mount type (e.g. "nfs", "9p", "virtiofs")
	Options string     // mount options (e.g. "rw,soft")
	Layer   string     // layer that declared this volume
}

// parseVolumeFile reads a volume declaration file and returns a Volume.
func parseVolumeFile(path, name, layerName string) (Volume, error) {
	f, err := os.Open(path)
	if err != nil {
		return Volume{}, err
	}
	defer f.Close()

	vol := Volume{
		Name:  name,
		Layer: layerName,
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "mount":
			vol.Mount = value
		case "size":
			vol.Size = value
		case "format":
			vol.Format = value
		case "owner":
			vol.Owner = value
		case "source":
			vol.Source = value
		case "type":
			vol.Type = value
		case "options":
			vol.Options = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Volume{}, err
	}

	// Determine kind and validate
	if name == "root" {
		vol.Kind = VolumeRoot
		if vol.Size == "" {
			return Volume{}, fmt.Errorf("volume %q in layer %s: root volume must have size:", name, layerName)
		}
		return vol, nil
	}

	if vol.Source != "" {
		// Network mount
		vol.Kind = VolumeNetwork
		if vol.Mount == "" {
			return Volume{}, fmt.Errorf("volume %q in layer %s: network mount must have mount:", name, layerName)
		}
		if vol.Type == "" {
			return Volume{}, fmt.Errorf("volume %q in layer %s: network mount must have type:", name, layerName)
		}
		return vol, nil
	}

	if vol.Size != "" {
		// Block volume
		vol.Kind = VolumeBlock
		if vol.Mount == "" {
			return Volume{}, fmt.Errorf("volume %q in layer %s: block volume must have mount:", name, layerName)
		}
		if vol.Format == "" {
			vol.Format = "ext4"
		}

		// Validate filesystem label length.
		// The label is "tank-" + name; ext4 supports max 16 chars, xfs max 12.
		label := "tank-" + name
		maxLabel := 0
		switch vol.Format {
		case "ext4":
			maxLabel = 16
		case "xfs":
			maxLabel = 12
		}
		if maxLabel > 0 && len(label) > maxLabel {
			return Volume{}, fmt.Errorf("volume %q in layer %s: label %q exceeds %s limit of %d characters", name, layerName, label, vol.Format, maxLabel)
		}

		return vol, nil
	}

	return Volume{}, fmt.Errorf("volume %q in layer %s: must have either size: or source:", name, layerName)
}

// loadLayerVolumes reads all volume declarations from a layer directory.
func loadLayerVolumes(layerPath, layerName string) ([]Volume, error) {
	volumesDir := filepath.Join(layerPath, "volumes")
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var volumes []Volume
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		vol, err := parseVolumeFile(filepath.Join(volumesDir, entry.Name()), entry.Name(), layerName)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, vol)
	}

	return volumes, nil
}

// CollectVolumes merges volumes from all layers.
// Returns block volumes, network mounts, the resolved root size, and any error.
// Duplicate non-root volume names across layers are an error.
// Multiple root declarations are merged by taking the maximum size.
func CollectVolumes(layers []Layer) (blocks []Volume, networks []Volume, rootSize string, err error) {
	seen := make(map[string]string) // volume name → layer name

	for _, layer := range layers {
		for _, vol := range layer.Volumes {
			if vol.Kind == VolumeRoot {
				if rootSize == "" || parseSizeBytes(vol.Size) > parseSizeBytes(rootSize) {
					rootSize = vol.Size
				}
				continue
			}

			if prevLayer, ok := seen[vol.Name]; ok {
				return nil, nil, "", fmt.Errorf("volume %q declared in both %s and %s", vol.Name, prevLayer, vol.Layer)
			}
			seen[vol.Name] = vol.Layer

			switch vol.Kind {
			case VolumeBlock:
				blocks = append(blocks, vol)
			case VolumeNetwork:
				networks = append(networks, vol)
			}
		}
	}

	return blocks, networks, rootSize, nil
}

// parseSizeBytes converts a human-readable size string (e.g. "20G", "512M", "1T")
// to bytes. Returns 0 for empty or unparseable strings.
func parseSizeBytes(s string) int64 {
	if s == "" {
		return 0
	}

	s = strings.TrimSpace(s)
	if len(s) < 2 {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return n
	}

	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}

	switch suffix {
	case 'K', 'k':
		return n * 1024
	case 'M', 'm':
		return n * 1024 * 1024
	case 'G', 'g':
		return n * 1024 * 1024 * 1024
	case 'T', 't':
		return n * 1024 * 1024 * 1024 * 1024
	default:
		// Maybe it's all digits
		full, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0
		}
		return full
	}
}
