package build

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneDryRunKeepsLatestProjectBuild(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	buildsDir := filepath.Join(tempDir, "builds")
	if err := os.MkdirAll(buildsDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error: %v", err)
	}

	for _, hash := range []string{"baseA", "oldA", "newA", "orphan"} {
		if err := os.WriteFile(filepath.Join(buildsDir, hash+".qcow2"), []byte(hash), 0644); err != nil {
			t.Fatalf("os.WriteFile() error: %v", err)
		}
	}

	meta := &artifactMetadata{
		Version: metadataVersion,
		Artifacts: map[string]artifactRecord{
			"baseA": {Hash: "baseA"},
			"oldA":  {Hash: "oldA", ParentHash: "baseA"},
			"newA":  {Hash: "newA", ParentHash: "baseA"},
		},
		Builds: []buildRecord{
			{ProjectRoot: "/project/a", ProjectName: "a", FinalHash: "oldA", CreatedAt: time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)},
			{ProjectRoot: "/project/a", ProjectName: "a", FinalHash: "newA", CreatedAt: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)},
		},
	}
	if err := saveMetadata(meta); err != nil {
		t.Fatalf("saveMetadata() error: %v", err)
	}

	var output bytes.Buffer
	result, err := Prune(&output, PruneOptions{})
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}

	if got := strings.Join(result.Roots, ","); got != "newA" {
		t.Fatalf("Roots = %q, want newA", got)
	}
	if got := strings.Join(result.Reachable, ","); got != "baseA,newA" {
		t.Fatalf("Reachable = %q, want baseA,newA", got)
	}
	if got := strings.Join(result.Reclaimable, ","); got != "oldA,orphan" {
		t.Fatalf("Reclaimable = %q, want oldA,orphan", got)
	}
	if !strings.Contains(output.String(), "Dry run") {
		t.Fatalf("output = %q, want dry-run summary", output.String())
	}
}

func TestPruneApplyDeletesUnreachableBuilds(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	buildsDir := filepath.Join(tempDir, "builds")
	if err := os.MkdirAll(buildsDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll() error: %v", err)
	}

	for _, hash := range []string{"baseA", "liveA", "deadA"} {
		if err := os.WriteFile(filepath.Join(buildsDir, hash+".qcow2"), []byte(hash), 0644); err != nil {
			t.Fatalf("os.WriteFile() error: %v", err)
		}
	}

	meta := &artifactMetadata{
		Version: metadataVersion,
		Artifacts: map[string]artifactRecord{
			"baseA": {Hash: "baseA"},
			"liveA": {Hash: "liveA", ParentHash: "baseA"},
			"deadA": {Hash: "deadA", ParentHash: "baseA"},
		},
		Builds: []buildRecord{
			{ProjectRoot: "/project/a", ProjectName: "a", FinalHash: "liveA", CreatedAt: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)},
		},
	}
	if err := saveMetadata(meta); err != nil {
		t.Fatalf("saveMetadata() error: %v", err)
	}

	var output bytes.Buffer
	result, err := Prune(&output, PruneOptions{Apply: true})
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}

	if got := strings.Join(result.Deleted, ","); got != "deadA" {
		t.Fatalf("Deleted = %q, want deadA", got)
	}
	if _, err := os.Stat(filepath.Join(buildsDir, "deadA.qcow2")); !os.IsNotExist(err) {
		t.Fatalf("deadA.qcow2 still exists")
	}
}

func TestPruneKeepsInstanceBackedBuildChain(t *testing.T) {
	if _, err := exec.LookPath("qemu-img"); err != nil {
		t.Skip("qemu-img not available")
	}

	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	buildsDir := filepath.Join(tempDir, "builds")
	instancesDir := filepath.Join(tempDir, "instances", "vm1")
	if err := os.MkdirAll(buildsDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll(buildsDir) error: %v", err)
	}
	if err := os.MkdirAll(instancesDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll(instancesDir) error: %v", err)
	}

	basePath := filepath.Join(buildsDir, "base.qcow2")
	stagePath := filepath.Join(buildsDir, "stage.qcow2")
	instancePath := filepath.Join(instancesDir, "disk.qcow2")

	runQemuImg(t, "create", "-f", "qcow2", basePath, "1M")
	runQemuImg(t, "create", "-f", "qcow2", "-F", "qcow2", "-b", basePath, stagePath)
	runQemuImg(t, "create", "-f", "qcow2", "-F", "qcow2", "-b", stagePath, instancePath)

	var output bytes.Buffer
	result, err := Prune(&output, PruneOptions{})
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}

	if got := strings.Join(result.Roots, ","); got != "stage" {
		t.Fatalf("Roots = %q, want stage", got)
	}
	if got := strings.Join(result.Reachable, ","); got != "base,stage" {
		t.Fatalf("Reachable = %q, want base,stage", got)
	}
	if len(result.Reclaimable) != 0 {
		t.Fatalf("Reclaimable = %v, want none", result.Reclaimable)
	}
}

func runQemuImg(t *testing.T, args ...string) {
	t.Helper()

	cmd := exec.Command("qemu-img", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("qemu-img %v error: %v: %s", args, err, output)
	}
}
