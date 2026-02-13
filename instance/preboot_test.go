package instance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rhettg/tank/project"
)

func TestValidateCloudInit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid with header",
			input: "#cloud-config\nusers:\n  - name: test\n",
			want:  "#cloud-config\nusers:\n  - name: test\n",
		},
		{
			name:  "missing header is prepended",
			input: "users:\n  - name: test\n",
			want:  "#cloud-config\nusers:\n  - name: test\n",
		},
		{
			name:    "empty content",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "  \n\n  ",
			wantErr: true,
		},
		{
			name:    "not yaml mapping",
			input:   "#cloud-config\n- item1\n- item2\n",
			wantErr: true,
		},
		{
			name:  "header only with body",
			input: "#cloud-config\nhostname: test\n",
			want:  "#cloud-config\nhostname: test\n",
		},
		{
			name:  "comments before keys",
			input: "#cloud-config\n# a comment\nusers:\n  - name: test\n",
			want:  "#cloud-config\n# a comment\nusers:\n  - name: test\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateCloudInit(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunPrebootHooks(t *testing.T) {
	// Create a temp project with a preboot hook
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a layer with preboot
	layerDir := filepath.Join(tmpDir, "layers", "10-test")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create preboot script that appends to cloud-init
	prebootScript := `#!/bin/bash
echo "# Added by preboot: $TANK_INSTANCE_NAME" >> "$TANK_CLOUD_INIT"
`
	if err := os.WriteFile(filepath.Join(layerDir, "preboot"), []byte(prebootScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Load project
	p, err := project.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Create temp cloud-init file
	cloudInitFile, err := os.CreateTemp("", "cloud-init-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(cloudInitFile.Name())

	initialContent := "#cloud-config\nusers: []\n"
	if _, err := cloudInitFile.WriteString(initialContent); err != nil {
		t.Fatal(err)
	}
	cloudInitFile.Close()

	// Run preboot hooks
	var output bytes.Buffer
	if err := RunPrebootHooks(p, "test-instance", cloudInitFile.Name(), &output); err != nil {
		t.Fatalf("RunPrebootHooks failed: %v", err)
	}

	// Read modified cloud-init
	modifiedContent, err := os.ReadFile(cloudInitFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Check that preboot hook modified the file
	if !strings.Contains(string(modifiedContent), "# Added by preboot: test-instance") {
		t.Errorf("preboot hook did not modify cloud-init, got:\n%s", modifiedContent)
	}
}

func TestRunPrebootHooksOrder(t *testing.T) {
	// Create a temp project with multiple preboot hooks
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create first layer
	layer1Dir := filepath.Join(tmpDir, "layers", "10-first")
	if err := os.MkdirAll(layer1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer1Dir, "preboot"), []byte("#!/bin/bash\necho 'first' >> \"$TANK_CLOUD_INIT\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create second layer
	layer2Dir := filepath.Join(tmpDir, "layers", "20-second")
	if err := os.MkdirAll(layer2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layer2Dir, "preboot"), []byte("#!/bin/bash\necho 'second' >> \"$TANK_CLOUD_INIT\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Load project
	p, err := project.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Create temp cloud-init file
	cloudInitFile, err := os.CreateTemp("", "cloud-init-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(cloudInitFile.Name())
	cloudInitFile.Close()

	// Run preboot hooks
	var output bytes.Buffer
	if err := RunPrebootHooks(p, "test-instance", cloudInitFile.Name(), &output); err != nil {
		t.Fatalf("RunPrebootHooks failed: %v", err)
	}

	// Read modified cloud-init
	modifiedContent, err := os.ReadFile(cloudInitFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Check order: first should appear before second
	content := string(modifiedContent)
	firstIdx := strings.Index(content, "first")
	secondIdx := strings.Index(content, "second")

	if firstIdx == -1 || secondIdx == -1 {
		t.Errorf("expected both 'first' and 'second' in output, got:\n%s", content)
	}
	if firstIdx > secondIdx {
		t.Errorf("hooks ran in wrong order, got:\n%s", content)
	}
}

func TestRunPrebootHooksFailure(t *testing.T) {
	// Create a temp project with a failing preboot hook
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a layer with failing preboot
	layerDir := filepath.Join(tmpDir, "layers", "10-test")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "preboot"), []byte("#!/bin/bash\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Load project
	p, err := project.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Create temp cloud-init file
	cloudInitFile, err := os.CreateTemp("", "cloud-init-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(cloudInitFile.Name())
	cloudInitFile.Close()

	// Run preboot hooks - should fail
	var output bytes.Buffer
	err = RunPrebootHooks(p, "test-instance", cloudInitFile.Name(), &output)
	if err == nil {
		t.Error("expected error from failing preboot hook")
	}
}

func TestRunPrebootHooksEnvVars(t *testing.T) {
	// Create a temp project with a preboot hook that outputs env vars
	tmpDir := t.TempDir()

	// Create BASE
	if err := os.WriteFile(filepath.Join(tmpDir, "BASE"), []byte("test-base\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a layer with preboot that writes env vars to cloud-init
	layerDir := filepath.Join(tmpDir, "layers", "10-test")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}
	prebootScript := `#!/bin/bash
echo "PROJECT_ROOT=$TANK_PROJECT_ROOT" >> "$TANK_CLOUD_INIT"
echo "INSTANCE_NAME=$TANK_INSTANCE_NAME" >> "$TANK_CLOUD_INIT"
echo "LAYER_PATH=$TANK_LAYER_PATH" >> "$TANK_CLOUD_INIT"
echo "WORK_DIR=$TANK_WORK_DIR" >> "$TANK_CLOUD_INIT"
# Verify work dir exists
test -d "$TANK_WORK_DIR" || exit 1
`
	if err := os.WriteFile(filepath.Join(layerDir, "preboot"), []byte(prebootScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Load project
	p, err := project.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Create temp cloud-init file
	cloudInitFile, err := os.CreateTemp("", "cloud-init-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(cloudInitFile.Name())
	cloudInitFile.Close()

	// Run preboot hooks
	var output bytes.Buffer
	if err := RunPrebootHooks(p, "my-instance", cloudInitFile.Name(), &output); err != nil {
		t.Fatalf("RunPrebootHooks failed: %v", err)
	}

	// Read modified cloud-init
	modifiedContent, err := os.ReadFile(cloudInitFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	content := string(modifiedContent)

	// Check env vars were set correctly
	if !strings.Contains(content, "PROJECT_ROOT="+p.Root) {
		t.Errorf("TANK_PROJECT_ROOT not set correctly, got:\n%s", content)
	}
	if !strings.Contains(content, "INSTANCE_NAME=my-instance") {
		t.Errorf("TANK_INSTANCE_NAME not set correctly, got:\n%s", content)
	}
	if !strings.Contains(content, "LAYER_PATH="+layerDir) {
		t.Errorf("TANK_LAYER_PATH not set correctly, got:\n%s", content)
	}
	if !strings.Contains(content, "WORK_DIR=/") {
		t.Errorf("TANK_WORK_DIR not set, got:\n%s", content)
	}
}
