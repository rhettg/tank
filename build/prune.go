package build

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type PruneOptions struct {
	Apply bool
}

type PruneResult struct {
	Roots            []string
	Reachable        []string
	Reclaimable      []string
	ReclaimableBytes int64
	Deleted          []string
	DeletedBytes     int64
}

func AnalyzePrune() (*PruneResult, error) {
	return prune(PruneOptions{})
}

func Prune(progress io.Writer, opts PruneOptions) (*PruneResult, error) {
	result, err := prune(opts)
	if err != nil {
		return nil, err
	}

	renderPruneResult(progress, result, opts.Apply)
	return result, nil
}

func prune(opts PruneOptions) (*PruneResult, error) {
	var result *PruneResult
	err := withMetadataLock(func() error {
		cacheDir, err := CacheDir()
		if err != nil {
			return err
		}

		buildsDir := filepath.Join(cacheDir, "builds")
		entries, err := os.ReadDir(buildsDir)
		if os.IsNotExist(err) {
			result = &PruneResult{}
			return nil
		}
		if err != nil {
			return err
		}

		buildFiles := make(map[string]string)
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".qcow2" {
				continue
			}
			hash := strings.TrimSuffix(entry.Name(), ".qcow2")
			buildFiles[hash] = filepath.Join(buildsDir, entry.Name())
		}

		meta, err := loadMetadata()
		if err != nil {
			return err
		}

		rootSet := make(map[string]struct{})
		for _, hash := range latestBuildRoots(meta.Builds) {
			if _, ok := buildFiles[hash]; ok {
				rootSet[hash] = struct{}{}
			}
		}

		instanceRoots, err := instanceBuildRoots(cacheDir, buildFiles)
		if err != nil {
			return err
		}
		for _, hash := range instanceRoots {
			rootSet[hash] = struct{}{}
		}

		var roots []string
		for hash := range rootSet {
			roots = append(roots, hash)
		}
		sort.Strings(roots)

		reachableSet := make(map[string]struct{})
		for _, hash := range roots {
			if err := markReachable(hash, buildFiles, meta.Artifacts, reachableSet); err != nil {
				return err
			}
		}

		result = &PruneResult{
			Roots: roots,
		}
		for hash := range reachableSet {
			result.Reachable = append(result.Reachable, hash)
		}
		sort.Strings(result.Reachable)

		for hash, path := range buildFiles {
			if _, ok := reachableSet[hash]; ok {
				continue
			}
			result.Reclaimable = append(result.Reclaimable, hash)

			info, err := os.Stat(path)
			if err == nil {
				result.ReclaimableBytes += info.Size()
			}

			if !opts.Apply {
				continue
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("removing %s: %w", path, err)
			}
			result.Deleted = append(result.Deleted, hash)
			if err == nil {
				result.DeletedBytes += info.Size()
			}
		}

		sort.Strings(result.Reclaimable)
		sort.Strings(result.Deleted)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func latestBuildRoots(builds []buildRecord) []string {
	latest := make(map[string]buildRecord)
	for _, record := range builds {
		current, ok := latest[record.ProjectRoot]
		if !ok || record.CreatedAt.After(current.CreatedAt) {
			latest[record.ProjectRoot] = record
		}
	}

	roots := make([]string, 0, len(latest))
	for _, record := range latest {
		roots = append(roots, record.FinalHash)
	}
	sort.Strings(roots)
	return roots
}

func instanceBuildRoots(cacheDir string, buildFiles map[string]string) ([]string, error) {
	instancesDir := filepath.Join(cacheDir, "instances")
	entries, err := os.ReadDir(instancesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	buildsDir := filepath.Join(cacheDir, "builds")
	roots := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		diskPath := filepath.Join(instancesDir, entry.Name(), "disk.qcow2")
		if _, err := os.Stat(diskPath); err != nil {
			continue
		}
		backingPath, err := qcow2BackingFile(diskPath)
		if err != nil {
			return nil, fmt.Errorf("inspecting instance %s: %w", entry.Name(), err)
		}
		hash, ok := buildHashFromPath(backingPath, buildsDir)
		if ok {
			if _, exists := buildFiles[hash]; exists {
				roots[hash] = struct{}{}
			}
		}
	}

	var result []string
	for hash := range roots {
		result = append(result, hash)
	}
	sort.Strings(result)
	return result, nil
}

func markReachable(hash string, buildFiles map[string]string, artifacts map[string]artifactRecord, reachable map[string]struct{}) error {
	if _, ok := reachable[hash]; ok {
		return nil
	}

	path, ok := buildFiles[hash]
	if !ok {
		return nil
	}
	reachable[hash] = struct{}{}

	if artifact, ok := artifacts[hash]; ok && artifact.ParentHash != "" {
		return markReachable(artifact.ParentHash, buildFiles, artifacts, reachable)
	}

	backingPath, err := qcow2BackingFile(path)
	if err != nil {
		return fmt.Errorf("inspecting build %s: %w", hash, err)
	}

	parentHash, ok := buildHashFromPath(backingPath, filepath.Dir(path))
	if !ok {
		return nil
	}
	return markReachable(parentHash, buildFiles, artifacts, reachable)
}

func buildHashFromPath(path string, buildsDir string) (string, bool) {
	if path == "" {
		return "", false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	absBuildsDir, err := filepath.Abs(buildsDir)
	if err != nil {
		return "", false
	}

	if filepath.Dir(absPath) != absBuildsDir || filepath.Ext(absPath) != ".qcow2" {
		return "", false
	}
	return strings.TrimSuffix(filepath.Base(absPath), ".qcow2"), true
}

func qcow2BackingFile(path string) (string, error) {
	cmd := exec.Command("qemu-img", "info", "--output=json", path)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("qemu-img info: %w", err)
	}

	var info struct {
		BackingFilename string `json:"backing-filename"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return "", fmt.Errorf("parsing qemu-img info output: %w", err)
	}

	return info.BackingFilename, nil
}

func renderPruneResult(w io.Writer, result *PruneResult, applied bool) {
	mode := "Dry run"
	if applied {
		mode = "Prune applied"
	}
	fmt.Fprintf(w, "%s\n", mode)
	fmt.Fprintf(w, "Roots: %d\n", len(result.Roots))
	fmt.Fprintf(w, "Reachable builds: %d\n", len(result.Reachable))
	fmt.Fprintf(w, "Reclaimable builds: %d (%s)\n", len(result.Reclaimable), formatBytes(result.ReclaimableBytes))
	if len(result.Reclaimable) > 0 {
		fmt.Fprintln(w, "")
		for _, hash := range result.Reclaimable {
			fmt.Fprintf(w, "  %s\n", hash)
		}
	}
	if applied {
		fmt.Fprintf(w, "\nDeleted: %d (%s)\n", len(result.Deleted), formatBytes(result.DeletedBytes))
	}
}
