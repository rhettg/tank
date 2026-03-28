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

	"github.com/rhettg/tank/ui"
)

type PruneOptions struct {
	Apply bool
}

type PruneResult struct {
	Roots            []string
	Pinned           []string
	Reachable        []string
	Reclaimable      []string
	ReclaimableBytes int64
	Deleted          []string
	DeletedBytes     int64
}

type pruneRoot struct {
	Hash   string
	Reason string
}

type PruneExplanation struct {
	Hash        string
	Kept        bool
	Reason      string
	Path        []string
	Reclaimable bool
}

func AnalyzePrune() (*PruneResult, error) {
	return prune(PruneOptions{})
}

func ExplainPrune(hash string) (*PruneExplanation, error) {
	var explanation *PruneExplanation
	err := withMetadataLock(func() error {
		analysis, err := analyzePruneState()
		if err != nil {
			return err
		}

		if _, ok := analysis.buildFiles[hash]; !ok {
			explanation = &PruneExplanation{Hash: hash}
			return nil
		}

		if _, ok := analysis.reachableSet[hash]; !ok {
			explanation = &PruneExplanation{
				Hash:        hash,
				Reclaimable: true,
			}
			return nil
		}

		for _, root := range analysis.roots {
			path, ok, err := tracePath(root.Hash, hash, analysis.buildFiles, analysis.meta.Artifacts)
			if err != nil {
				return err
			}
			if ok {
				explanation = &PruneExplanation{
					Hash:   hash,
					Kept:   true,
					Reason: root.Reason,
					Path:   path,
				}
				return nil
			}
		}

		explanation = &PruneExplanation{Hash: hash, Kept: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return explanation, nil
}

func Prune(progress io.Writer, opts PruneOptions) (*PruneResult, error) {
	result, err := prune(opts)
	if err != nil {
		return nil, err
	}

	renderPruneResult(progress, result, opts.Apply)
	return result, nil
}

func AutoPrune(progress io.Writer) (*PruneResult, error) {
	result, err := prune(PruneOptions{Apply: true})
	if err != nil {
		return nil, err
	}
	if len(result.Deleted) > 0 {
		fmt.Fprintf(progress, "%s Reclaimed %d cached build(s) (%s)\n",
			ui.SymbolSuccess,
			len(result.Deleted),
			formatBytes(result.DeletedBytes),
		)
	}
	return result, nil
}

func prune(opts PruneOptions) (*PruneResult, error) {
	var result *PruneResult
	err := withMetadataLock(func() error {
		analysis, err := analyzePruneState()
		if err != nil {
			return err
		}
		if analysis == nil {
			result = &PruneResult{}
			return nil
		}

		result = &PruneResult{
			Roots:  analysis.rootHashes(),
			Pinned: append([]string(nil), analysis.meta.Pins...),
		}
		for hash := range analysis.reachableSet {
			result.Reachable = append(result.Reachable, hash)
		}
		sort.Strings(result.Reachable)

		for hash, path := range analysis.buildFiles {
			if _, ok := analysis.reachableSet[hash]; ok {
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

type pruneAnalysis struct {
	meta         *artifactMetadata
	buildFiles   map[string]string
	roots        []pruneRoot
	reachableSet map[string]struct{}
}

func (p *pruneAnalysis) rootHashes() []string {
	hashes := make([]string, 0, len(p.roots))
	for _, root := range p.roots {
		hashes = append(hashes, root.Hash)
	}
	sort.Strings(hashes)
	return hashes
}

func analyzePruneState() (*pruneAnalysis, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return nil, err
	}

	buildsDir := filepath.Join(cacheDir, "builds")
	entries, err := os.ReadDir(buildsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
		return nil, err
	}

	rootsByHash := make(map[string]pruneRoot)
	for _, root := range latestBuildRoots(meta.Builds) {
		if _, ok := buildFiles[root.Hash]; ok {
			rootsByHash[root.Hash] = root
		}
	}

	instanceRoots, err := instanceBuildRoots(cacheDir, buildFiles)
	if err != nil {
		return nil, err
	}
	for _, root := range instanceRoots {
		if existing, ok := rootsByHash[root.Hash]; ok {
			existing.Reason = existing.Reason + "; " + root.Reason
			rootsByHash[root.Hash] = existing
			continue
		}
		rootsByHash[root.Hash] = root
	}
	for _, root := range pinnedRoots(meta.Pins) {
		if _, ok := buildFiles[root.Hash]; !ok {
			continue
		}
		if existing, ok := rootsByHash[root.Hash]; ok {
			existing.Reason = existing.Reason + "; " + root.Reason
			rootsByHash[root.Hash] = existing
			continue
		}
		rootsByHash[root.Hash] = root
	}

	var roots []pruneRoot
	for _, root := range rootsByHash {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Hash < roots[j].Hash
	})

	reachableSet := make(map[string]struct{})
	for _, root := range roots {
		if err := markReachable(root.Hash, buildFiles, meta.Artifacts, reachableSet); err != nil {
			return nil, err
		}
	}

	return &pruneAnalysis{
		meta:         meta,
		buildFiles:   buildFiles,
		roots:        roots,
		reachableSet: reachableSet,
	}, nil
}

func latestBuildRoots(builds []buildRecord) []pruneRoot {
	latest := make(map[string]buildRecord)
	for _, record := range builds {
		if _, err := os.Stat(record.ProjectRoot); err != nil {
			continue
		}
		current, ok := latest[record.ProjectRoot]
		if !ok || record.CreatedAt.After(current.CreatedAt) {
			latest[record.ProjectRoot] = record
		}
	}

	roots := make([]pruneRoot, 0, len(latest))
	for _, record := range latest {
		reason := fmt.Sprintf("latest build for project %s", record.ProjectName)
		roots = append(roots, pruneRoot{Hash: record.FinalHash, Reason: reason})
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Hash < roots[j].Hash
	})
	return roots
}

func pinnedRoots(pins []string) []pruneRoot {
	roots := make([]pruneRoot, 0, len(pins))
	for _, hash := range pins {
		roots = append(roots, pruneRoot{
			Hash:   hash,
			Reason: "pinned by user",
		})
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Hash < roots[j].Hash
	})
	return roots
}

func instanceBuildRoots(cacheDir string, buildFiles map[string]string) ([]pruneRoot, error) {
	instancesDir := filepath.Join(cacheDir, "instances")
	entries, err := os.ReadDir(instancesDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	buildsDir := filepath.Join(cacheDir, "builds")
	roots := make(map[string]pruneRoot)
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
				reason := fmt.Sprintf("backing file for instance %s", entry.Name())
				if existing, ok := roots[hash]; ok {
					existing.Reason = existing.Reason + "; " + reason
					roots[hash] = existing
				} else {
					roots[hash] = pruneRoot{Hash: hash, Reason: reason}
				}
			}
		}
	}

	var result []pruneRoot
	for _, root := range roots {
		result = append(result, root)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Hash < result[j].Hash
	})
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

	if artifact, ok := artifacts[hash]; ok {
		if artifact.ParentHash == "" {
			return nil
		}
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

func tracePath(currentHash, targetHash string, buildFiles map[string]string, artifacts map[string]artifactRecord) ([]string, bool, error) {
	path := []string{currentHash}
	for {
		if currentHash == targetHash {
			return path, true, nil
		}

		parentHash, ok, err := parentHashFor(currentHash, buildFiles, artifacts)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		path = append(path, parentHash)
		currentHash = parentHash
	}
}

func parentHashFor(hash string, buildFiles map[string]string, artifacts map[string]artifactRecord) (string, bool, error) {
	path, ok := buildFiles[hash]
	if !ok {
		return "", false, nil
	}
	if artifact, ok := artifacts[hash]; ok {
		if artifact.ParentHash == "" {
			return "", false, nil
		}
		return artifact.ParentHash, true, nil
	}
	backingPath, err := qcow2BackingFile(path)
	if err != nil {
		return "", false, fmt.Errorf("inspecting build %s: %w", hash, err)
	}
	parentHash, ok := buildHashFromPath(backingPath, filepath.Dir(path))
	return parentHash, ok, nil
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
	cmd := exec.Command("qemu-img", "info", "-U", "--output=json", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("qemu-img info -U %s: %w: %s", path, err, strings.TrimSpace(string(output)))
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
	ui.PrintHeader(w, "Prune")

	modeLabel := ui.Highlight.Render("dry run")
	if applied {
		modeLabel = ui.SuccessStyle.Render("applied")
	}
	fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Mode:"), modeLabel)
	fmt.Fprintf(w, "%s %d\n", ui.Bold.Render("Roots:"), len(result.Roots))
	if len(result.Pinned) > 0 {
		fmt.Fprintf(w, "%s %d\n", ui.Bold.Render("Pinned builds:"), len(result.Pinned))
	}
	fmt.Fprintf(w, "%s %d\n", ui.Bold.Render("Reachable builds:"), len(result.Reachable))

	if len(result.Reclaimable) == 0 {
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Reclaimable builds:"), ui.SuccessStyle.Render("none"))
	} else {
		fmt.Fprintf(w, "%s %s\n",
			ui.Bold.Render("Reclaimable builds:"),
			ui.WarningStyle.Render(fmt.Sprintf("%d (%s)", len(result.Reclaimable), formatBytes(result.ReclaimableBytes))),
		)
	}

	if applied {
		reclaimedLabel := ui.SuccessStyle.Render(fmt.Sprintf("%d (%s)", len(result.Deleted), formatBytes(result.DeletedBytes)))
		if len(result.Deleted) == 0 {
			reclaimedLabel = ui.MutedStyle.Render("0")
		}
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Reclaimed:"), reclaimedLabel)
	}

	if len(result.Reclaimable) == 0 {
		return
	}

	sectionTitle := "Reclaimable builds:"
	if applied {
		sectionTitle = "Removed builds:"
	}
	fmt.Fprintf(w, "\n%s\n", ui.Bold.Render(sectionTitle))
	for _, hash := range result.Reclaimable {
		fmt.Fprintf(w, "  %s %s\n", ui.SymbolDot, ui.MutedStyle.Render(hash))
	}
}

func RenderPruneExplanation(w io.Writer, explanation *PruneExplanation) {
	switch {
	case explanation == nil || explanation.Hash == "":
		ui.PrintInfo(w, "No build hash specified")
	case explanation.Kept:
		ui.PrintHeader(w, "Prune Explain")
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Build:"), ui.MutedStyle.Render(explanation.Hash))
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("State:"), ui.SuccessStyle.Render("kept"))
		if explanation.Reason != "" {
			fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Reason:"), explanation.Reason)
		}
		if len(explanation.Path) > 0 {
			fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Path:"), ui.MutedStyle.Render(strings.Join(explanation.Path, " -> ")))
		}
	case explanation.Reclaimable:
		ui.PrintHeader(w, "Prune Explain")
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Build:"), ui.MutedStyle.Render(explanation.Hash))
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("State:"), ui.WarningStyle.Render("reclaimable"))
	default:
		ui.PrintHeader(w, "Prune Explain")
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("Build:"), ui.MutedStyle.Render(explanation.Hash))
		fmt.Fprintf(w, "%s %s\n", ui.Bold.Render("State:"), ui.MutedStyle.Render("not present in cache"))
	}
}
