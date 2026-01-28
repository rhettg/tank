package build

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhettg/graystone/project"
)

func TestCacheDir(t *testing.T) {
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}

	if dir != "/var/lib/graystone" {
		t.Errorf("CacheDir() = %q, want /var/lib/graystone", dir)
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
	if !strings.Contains(output, "Layers (3)") {
		t.Error("output missing 'Layers (3)' section")
	}
	if !strings.Contains(output, "10-common") {
		t.Error("output missing 10-common layer")
	}
	if !strings.Contains(output, "20-devtools") {
		t.Error("output missing 20-devtools layer")
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
	if !strings.Contains(output, "run") || !strings.Contains(output, "install.sh") {
		t.Error("output missing 'run install.sh' step")
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
	path, err := BaseImagePath("https://example.com/images/test.img")
	if err != nil {
		t.Fatalf("BaseImagePath() error: %v", err)
	}

	if path != "/var/lib/graystone/images/test.img" {
		t.Errorf("BaseImagePath() = %q, want /var/lib/graystone/images/test.img", path)
	}
}

func TestDownloadBaseImage(t *testing.T) {
	// Ensure we can write to the storage directory
	if err := os.MkdirAll("/var/lib/graystone/images", 0755); err != nil {
		t.Skipf("cannot create storage directory (need write access to /var/lib/graystone): %v", err)
	}

	// Create a test server
	testContent := []byte("test image content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "18")
		w.Write(testContent)
	}))
	defer server.Close()

	// Clean up test image after test
	imagePath := "/var/lib/graystone/images/test-download.img"
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
	if err := os.MkdirAll("/var/lib/graystone/images", 0755); err != nil {
		t.Skipf("cannot create storage directory (need write access to /var/lib/graystone): %v", err)
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
	path, err := BuildImagePath("abc123")
	if err != nil {
		t.Fatalf("BuildImagePath() error: %v", err)
	}

	if path != "/var/lib/graystone/builds/abc123.qcow2" {
		t.Errorf("BuildImagePath() = %q, want /var/lib/graystone/builds/abc123.qcow2", path)
	}
}

func TestBuildImageExists(t *testing.T) {
	// Non-existent build should return false
	exists := BuildImageExists("nonexistent-hash-12345")
	if exists {
		t.Error("BuildImageExists() = true for non-existent build, want false")
	}
}

func TestCreateBuildImage(t *testing.T) {
	if err := os.MkdirAll("/var/lib/graystone/builds", 0755); err != nil {
		t.Skipf("cannot create storage directory (need write access to /var/lib/graystone): %v", err)
	}

	// Create a fake base image in a temp directory
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)
	baseImagePath := filepath.Join(baseDir, "test.qcow2")
	testContent := []byte("fake qcow2 image content")
	if err := os.WriteFile(baseImagePath, testContent, 0644); err != nil {
		t.Fatalf("writing test base image: %v", err)
	}

	// Clean up build image after test
	projectHash := "testhash123"
	defer os.Remove("/var/lib/graystone/builds/" + projectHash + ".qcow2")

	// Create build image
	var progress bytes.Buffer
	buildPath, err := CreateBuildImage(baseImagePath, projectHash, &progress)
	if err != nil {
		t.Fatalf("CreateBuildImage() error: %v", err)
	}

	// Verify build image was created
	content, err := os.ReadFile(buildPath)
	if err != nil {
		t.Fatalf("reading build image: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("build image content = %q, want %q", content, testContent)
	}

	// Verify progress output
	if !strings.Contains(progress.String(), "Source:") {
		t.Error("progress missing 'Source:'")
	}
	if !strings.Contains(progress.String(), "Created:") {
		t.Error("progress missing 'Created:'")
	}

	// Test cached case
	progress.Reset()
	cachedPath, err := CreateBuildImage(baseImagePath, projectHash, &progress)
	if err != nil {
		t.Fatalf("CreateBuildImage() cached error: %v", err)
	}
	if cachedPath != buildPath {
		t.Errorf("cached path = %q, want %q", cachedPath, buildPath)
	}
	if !strings.Contains(progress.String(), "Using existing build image") {
		t.Error("progress missing 'Using existing build image' for cached build")
	}
}
