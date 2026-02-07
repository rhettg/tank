package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	p, err := Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check base
	expected := "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
	if p.Base != expected {
		t.Errorf("Base = %q, want %q", p.Base, expected)
	}

	// Check cloud-init exists
	if p.CloudInit == "" {
		t.Error("CloudInit should not be empty")
	}

	// Check layers
	if len(p.Layers) != 4 {
		t.Fatalf("got %d layers, want 4", len(p.Layers))
	}

	// Check layer order and properties
	tests := []struct {
		name         string
		hasScript    bool
		hasFiles     bool
		hasFirstboot bool
		hasPreboot   bool
	}{
		{"10-common", true, true, false, false},
		{"20-devtools", true, false, false, false},
		{"50-preboot-test", false, false, false, true},
		{"90-project", false, true, false, false},
	}

	for i, tt := range tests {
		layer := p.Layers[i]
		if layer.Name != tt.name {
			t.Errorf("layer[%d].Name = %q, want %q", i, layer.Name, tt.name)
		}
		if layer.HasScript != tt.hasScript {
			t.Errorf("layer[%d].HasScript = %v, want %v", i, layer.HasScript, tt.hasScript)
		}
		if layer.HasFiles != tt.hasFiles {
			t.Errorf("layer[%d].HasFiles = %v, want %v", i, layer.HasFiles, tt.hasFiles)
		}
		if layer.HasFirstboot != tt.hasFirstboot {
			t.Errorf("layer[%d].HasFirstboot = %v, want %v", i, layer.HasFirstboot, tt.hasFirstboot)
		}
		if layer.HasPreboot != tt.hasPreboot {
			t.Errorf("layer[%d].HasPreboot = %v, want %v", i, layer.HasPreboot, tt.hasPreboot)
		}
		if layer.ContentHash == "" {
			t.Errorf("layer[%d].ContentHash should not be empty", i)
		}
	}
}

func TestHashDeterminism(t *testing.T) {
	// Load twice and ensure hashes match
	p1, err := Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("first Load failed: %v", err)
	}

	p2, err := Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("second Load failed: %v", err)
	}

	for i := range p1.Layers {
		if p1.Layers[i].ContentHash != p2.Layers[i].ContentHash {
			t.Errorf("layer %s: hash mismatch between loads", p1.Layers[i].Name)
		}
	}
}

func TestHashChangesOnModification(t *testing.T) {
	// Create a temp project
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a layer with install
	layerDir := filepath.Join(tmpDir, "layers", "10-test")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(layerDir, "install")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho hello\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Load and get hash
	p1, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("first Load failed: %v", err)
	}
	hash1 := p1.Layers[0].ContentHash

	// Modify the script
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho modified\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Load and get new hash
	p2, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("second Load failed: %v", err)
	}
	hash2 := p2.Layers[0].ContentHash

	if hash1 == hash2 {
		t.Error("hash should change after script modification")
	}
}

func TestProjectHash(t *testing.T) {
	p, err := Load("../testdata/example-project")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hash := p.Hash()

	// Hash should be 64 hex characters (SHA256)
	if len(hash) != 64 {
		t.Errorf("Hash() length = %d, want 64", len(hash))
	}

	// Hash should be deterministic
	hash2 := p.Hash()
	if hash != hash2 {
		t.Error("Hash() not deterministic")
	}
}

func TestProjectHashChanges(t *testing.T) {
	// Create a temp project
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a layer
	layerDir := filepath.Join(tmpDir, "layers", "10-test")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "install"), []byte("echo hello\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Load and get project hash
	p1, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("first Load failed: %v", err)
	}
	hash1 := p1.Hash()

	// Modify the base
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("different-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p2, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("second Load failed: %v", err)
	}
	hash2 := p2.Hash()

	if hash1 == hash2 {
		t.Error("project hash should change when BASE changes")
	}
}
