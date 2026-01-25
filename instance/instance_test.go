package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceDir(t *testing.T) {
	dir, err := InstanceDir("test-instance")
	if err != nil {
		t.Fatalf("InstanceDir() error: %v", err)
	}

	if !strings.HasSuffix(dir, ".cache/graystone/instances/test-instance") {
		t.Errorf("InstanceDir() = %q, want suffix .cache/graystone/instances/test-instance", dir)
	}
}

func TestExists(t *testing.T) {
	// Non-existent instance should return false
	exists := Exists("nonexistent-instance-12345")
	if exists {
		t.Error("Exists() = true for non-existent instance, want false")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("nonexistent-instance-12345")
	if err == nil {
		t.Error("Load() expected error for non-existent instance, got nil")
	}
}

func TestDomainName(t *testing.T) {
	// Use a temp directory for the cache
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a fake instance directory
	instanceDir := filepath.Join(tmpDir, ".cache", "graystone", "instances", "myproject")
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create disk file so Load doesn't fail
	if err := os.WriteFile(filepath.Join(instanceDir, "disk.qcow2"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	inst, err := Load("myproject")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if inst.Domain != "gi-myproject" {
		t.Errorf("Domain = %q, want %q", inst.Domain, "gi-myproject")
	}
}
