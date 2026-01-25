package build

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rhettg/graystone/project"
)

func TestCacheDir(t *testing.T) {
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error: %v", err)
	}

	if !strings.HasSuffix(dir, ".cache/graystone") {
		t.Errorf("CacheDir() = %q, want suffix .cache/graystone", dir)
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
	if !strings.Contains(output, "Base Image:") {
		t.Error("output missing 'Base Image:' section")
	}
	if !strings.Contains(output, "noble-server-cloudimg") {
		t.Error("output missing base image URL")
	}

	// Check layers section
	if !strings.Contains(output, "Layers (3):") {
		t.Error("output missing 'Layers (3):' section")
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
	if !strings.Contains(output, "Copy files/etc/motd -> /etc/motd") {
		t.Error("output missing file copy for /etc/motd")
	}
	if !strings.Contains(output, "Copy files/opt/app/README -> /opt/app/README") {
		t.Error("output missing file copy for /opt/app/README")
	}

	// Check script steps
	if !strings.Contains(output, "Run install.sh") {
		t.Error("output missing 'Run install.sh' step")
	}

	// Check cloud-init section
	if !strings.Contains(output, "Cloud-Init:") {
		t.Error("output missing 'Cloud-Init:' section")
	}
	if !strings.Contains(output, "Inject cloud-init.yaml") {
		t.Error("output missing cloud-init injection step")
	}

	// Check output section
	if !strings.Contains(output, "Output:") {
		t.Error("output missing 'Output:' section")
	}
	if !strings.Contains(output, ".qcow2") {
		t.Error("output missing .qcow2 extension")
	}
}
