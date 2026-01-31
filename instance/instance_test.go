package instance

import (
	"os"
	"testing"
)

func TestInstanceDir(t *testing.T) {
	dir, err := InstanceDir("test-instance")
	if err != nil {
		t.Fatalf("InstanceDir() error: %v", err)
	}

	if dir != "/var/lib/tank/instances/test-instance" {
		t.Errorf("InstanceDir() = %q, want /var/lib/tank/instances/test-instance", dir)
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
	// Create a fake instance directory at the expected path
	instanceDir := "/var/lib/tank/instances/myproject"
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		t.Skipf("cannot create test directory (need write access to /var/lib/tank): %v", err)
	}
	defer os.RemoveAll(instanceDir)

	// Create disk file so Load doesn't fail
	if err := os.WriteFile(instanceDir+"/disk.qcow2", []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	inst, err := Load("myproject")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if inst.Domain != "tank-myproject" {
		t.Errorf("Domain = %q, want %q", inst.Domain, "tank-myproject")
	}
}
