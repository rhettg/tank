package build

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rhettg/tank/project"
)

func TestCacheDir(t *testing.T) {
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}

	if dir != "/var/lib/tank" {
		t.Errorf("CacheDir() = %q, want /var/lib/tank", dir)
	}
}

func TestCacheDirOverride(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}

	if dir != tempDir {
		t.Errorf("CacheDir() = %q, want %q", dir, tempDir)
	}
}

func TestBaseImageCached(t *testing.T) {
	// Non-existent image should not be cached
	cached := BaseImageCached("https://example.com/nonexistent.img")
	if cached {
		t.Error("BaseImageCached() = true for non-existent image, want false")
	}
}

func TestPrintPlan(t *testing.T) {
	p, err := project.Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("project.Load() error: %v", err)
	}

	var buf bytes.Buffer
	if err := PrintPlan(&buf, p); err != nil {
		t.Fatalf("PrintPlan() error: %v", err)
	}

	output := buf.String()

	// Check base image section
	if !strings.Contains(output, "Base Image") {
		t.Error("output missing 'Base Image' section")
	}
	if !strings.Contains(output, "noble-server-cloudimg") {
		t.Error("output missing base image URL")
	}

	// Check layers section
	if !strings.Contains(output, "Layers (5)") {
		t.Error("output missing 'Layers (5)' section")
	}
	if !strings.Contains(output, "10-common") {
		t.Error("output missing 10-common layer")
	}
	if !strings.Contains(output, "20-devtools") {
		t.Error("output missing 20-devtools layer")
	}
	if !strings.Contains(output, "20-user-ssh") {
		t.Error("output missing 20-user-ssh layer")
	}
	if !strings.Contains(output, "90-project") {
		t.Error("output missing 90-project layer")
	}

	// Check file copy steps
	if !strings.Contains(output, "copy") || !strings.Contains(output, "/etc/motd") {
		t.Error("output missing file copy for /etc/motd")
	}
	if !strings.Contains(output, "copy") || !strings.Contains(output, "/opt/app/README") {
		t.Error("output missing file copy for /opt/app/README")
	}

	// Check script steps
	if !strings.Contains(output, "run") || !strings.Contains(output, "install") {
		t.Error("output missing 'run install' step")
	}

	// Check cloud-init section
	if !strings.Contains(output, "Cloud-Init") {
		t.Error("output missing 'Cloud-Init' section")
	}
	if !strings.Contains(output, "inject cloud-init.yaml") {
		t.Error("output missing cloud-init injection step")
	}

	// Check output section
	if !strings.Contains(output, "Output") {
		t.Error("output missing 'Output' section")
	}
	if !strings.Contains(output, ".qcow2") {
		t.Error("output missing .qcow2 extension")
	}
}

func TestBaseImagePath(t *testing.T) {
	t.Setenv("TANK_CACHE_DIR", "/var/lib/tank")

	path, err := BaseImagePath("https://example.com/images/test.img")
	if err != nil {
		t.Fatalf("BaseImagePath() error: %v", err)
	}

	if path != "/var/lib/tank/images/test.img" {
		t.Errorf("BaseImagePath() = %q, want /var/lib/tank/images/test.img", path)
	}
}

func TestDownloadBaseImage(t *testing.T) {
	// Ensure we can write to the storage directory
	if err := os.MkdirAll("/var/lib/tank/images", 0755); err != nil {
		t.Skipf("cannot create storage directory (need write access to /var/lib/tank): %v", err)
	}

	// Create a test server
	testContent := []byte("test image content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "18")
		w.Write(testContent)
	}))
	defer server.Close()

	// Clean up test image after test
	imagePath := "/var/lib/tank/images/test-download.img"
	defer os.Remove(imagePath)

	// Download the image
	var progress bytes.Buffer
	gotPath, err := DownloadBaseImage(server.URL+"/test-download.img", &progress)
	if err != nil {
		t.Fatalf("DownloadBaseImage() error: %v", err)
	}

	// Verify the file was downloaded
	content, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("downloaded content = %q, want %q", content, testContent)
	}

	// Verify progress output contains the download URL
	if !strings.Contains(progress.String(), "127.0.0.1") && !strings.Contains(progress.String(), "Saved") {
		t.Error("progress output missing URL or Saved indicator")
	}

	// Test cached case - should return immediately
	progress.Reset()
	cachedPath, err := DownloadBaseImage(server.URL+"/test-download.img", &progress)
	if err != nil {
		t.Fatalf("DownloadBaseImage() cached error: %v", err)
	}
	if cachedPath != gotPath {
		t.Errorf("cached path = %q, want %q", cachedPath, gotPath)
	}
	if !strings.Contains(progress.String(), "Using cached image") {
		t.Error("progress output missing 'Using cached image' for cached file")
	}
}

func TestDownloadBaseImageNotFound(t *testing.T) {
	if err := os.MkdirAll("/var/lib/tank/images", 0755); err != nil {
		t.Skipf("cannot create storage directory (need write access to /var/lib/tank): %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	var progress bytes.Buffer
	_, err := DownloadBaseImage(server.URL+"/notfound.img", &progress)
	if err == nil {
		t.Error("DownloadBaseImage() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want to contain '404'", err.Error())
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestBuildImagePath(t *testing.T) {
	t.Setenv("TANK_CACHE_DIR", "/var/lib/tank")

	path, err := BuildImagePath("abc123")
	if err != nil {
		t.Fatalf("BuildImagePath() error: %v", err)
	}

	if path != "/var/lib/tank/builds/abc123.qcow2" {
		t.Errorf("BuildImagePath() = %q, want /var/lib/tank/builds/abc123.qcow2", path)
	}
}

func TestBuildImageExists(t *testing.T) {
	// Non-existent build should return false
	exists := BuildImageExists("nonexistent-hash-12345")
	if exists {
		t.Error("BuildImageExists() = true for non-existent build, want false")
	}
}

func TestRecordBuildArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	p, err := project.Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("project.Load() error: %v", err)
	}

	stages := p.BuildChain("40G")
	finalHash := stages[len(stages)-1].Hash

	if err := recordBuildArtifacts(p, stages, finalHash); err != nil {
		t.Fatalf("recordBuildArtifacts() error: %v", err)
	}

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error: %v", err)
	}

	if len(meta.Artifacts) != len(stages) {
		t.Fatalf("len(meta.Artifacts) = %d, want %d", len(meta.Artifacts), len(stages))
	}

	for i, stage := range stages {
		record, ok := meta.Artifacts[stage.Hash]
		if !ok {
			t.Fatalf("missing artifact record for %s", stage.Hash)
		}
		if i == 0 {
			if record.ParentHash != "" {
				t.Fatalf("base stage parent = %q, want empty", record.ParentHash)
			}
			continue
		}
		if record.ParentHash != stages[i-1].Hash {
			t.Fatalf("record.ParentHash = %q, want %q", record.ParentHash, stages[i-1].Hash)
		}
	}

	if len(meta.Builds) != 1 {
		t.Fatalf("len(meta.Builds) = %d, want 1", len(meta.Builds))
	}
	if meta.Builds[0].ProjectRoot != p.Root {
		t.Fatalf("ProjectRoot = %q, want %q", meta.Builds[0].ProjectRoot, p.Root)
	}
	if meta.Builds[0].ProjectName != filepath.Base(p.Root) {
		t.Fatalf("ProjectName = %q, want %q", meta.Builds[0].ProjectName, filepath.Base(p.Root))
	}
	if meta.Builds[0].FinalHash != finalHash {
		t.Fatalf("FinalHash = %q, want %q", meta.Builds[0].FinalHash, finalHash)
	}
}

func TestRecordBuildArtifactsConcurrent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	p1, err := project.Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("project.Load() error: %v", err)
	}
	p2 := *p1
	p2.Root = filepath.Join(tempDir, "project-two")

	stages1 := p1.BuildChain("40G")
	stages2 := p2.BuildChain("40G")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := recordBuildArtifacts(p1, stages1, stages1[len(stages1)-1].Hash); err != nil {
			t.Errorf("recordBuildArtifacts(p1) error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := recordBuildArtifacts(&p2, stages2, stages2[len(stages2)-1].Hash); err != nil {
			t.Errorf("recordBuildArtifacts(p2) error: %v", err)
		}
	}()
	wg.Wait()

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error: %v", err)
	}
	if len(meta.Builds) != 2 {
		t.Fatalf("len(meta.Builds) = %d, want 2", len(meta.Builds))
	}
}

func TestPinAndUnpinBuild(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TANK_CACHE_DIR", tempDir)

	if err := PinBuild("abc123"); err != nil {
		t.Fatalf("PinBuild() error: %v", err)
	}
	if err := PinBuild("abc123"); err != nil {
		t.Fatalf("PinBuild() duplicate error: %v", err)
	}

	pinned, err := IsPinned("abc123")
	if err != nil {
		t.Fatalf("IsPinned() error: %v", err)
	}
	if !pinned {
		t.Fatalf("IsPinned() = false, want true")
	}

	meta, err := loadMetadata()
	if err != nil {
		t.Fatalf("loadMetadata() error: %v", err)
	}
	if got := len(meta.Pins); got != 1 {
		t.Fatalf("len(meta.Pins) = %d, want 1", got)
	}

	if err := UnpinBuild("abc123"); err != nil {
		t.Fatalf("UnpinBuild() error: %v", err)
	}
	pinned, err = IsPinned("abc123")
	if err != nil {
		t.Fatalf("IsPinned() after unpin error: %v", err)
	}
	if pinned {
		t.Fatalf("IsPinned() = true after unpin, want false")
	}
}

func TestAutoPruneDeletesReclaimableBuilds(t *testing.T) {
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
			{ProjectRoot: "/project/a", ProjectName: "a", FinalHash: "liveA"},
		},
	}
	if err := saveMetadata(meta); err != nil {
		t.Fatalf("saveMetadata() error: %v", err)
	}

	var progress bytes.Buffer
	result, err := AutoPrune(&progress)
	if err != nil {
		t.Fatalf("AutoPrune() error: %v", err)
	}
	if got := strings.Join(result.Deleted, ","); got != "deadA" {
		t.Fatalf("Deleted = %q, want deadA", got)
	}
	if !strings.Contains(progress.String(), "Reclaimed 1 cached build(s)") {
		t.Fatalf("progress = %q, want reclaim summary", progress.String())
	}
}
