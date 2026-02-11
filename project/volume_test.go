package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBlockVolume(t *testing.T) {
	tmpDir := t.TempDir()
	volFile := filepath.Join(tmpDir, "pgdata")
	content := "mount: /var/lib/postgresql\nsize: 20G\nformat: ext4\nowner: postgres:postgres\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "pgdata", "50-postgres")
	if err != nil {
		t.Fatalf("parseVolumeFile: %v", err)
	}

	if vol.Kind != VolumeBlock {
		t.Errorf("Kind = %v, want VolumeBlock", vol.Kind)
	}
	if vol.Name != "pgdata" {
		t.Errorf("Name = %q, want %q", vol.Name, "pgdata")
	}
	if vol.Mount != "/var/lib/postgresql" {
		t.Errorf("Mount = %q, want %q", vol.Mount, "/var/lib/postgresql")
	}
	if vol.Size != "20G" {
		t.Errorf("Size = %q, want %q", vol.Size, "20G")
	}
	if vol.Format != "ext4" {
		t.Errorf("Format = %q, want %q", vol.Format, "ext4")
	}
	if vol.Owner != "postgres:postgres" {
		t.Errorf("Owner = %q, want %q", vol.Owner, "postgres:postgres")
	}
	if vol.Layer != "50-postgres" {
		t.Errorf("Layer = %q, want %q", vol.Layer, "50-postgres")
	}
}

func TestParseBlockVolumeDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	volFile := filepath.Join(tmpDir, "data")
	content := "mount: /data\nsize: 10G\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "data", "10-base")
	if err != nil {
		t.Fatalf("parseVolumeFile: %v", err)
	}

	if vol.Format != "ext4" {
		t.Errorf("Format = %q, want default %q", vol.Format, "ext4")
	}
	if vol.Owner != "" {
		t.Errorf("Owner = %q, want empty", vol.Owner)
	}
}

func TestParseRootVolume(t *testing.T) {
	tmpDir := t.TempDir()
	volFile := filepath.Join(tmpDir, "root")
	content := "size: 200G\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "root", "50-big-models")
	if err != nil {
		t.Fatalf("parseVolumeFile: %v", err)
	}

	if vol.Kind != VolumeRoot {
		t.Errorf("Kind = %v, want VolumeRoot", vol.Kind)
	}
	if vol.Size != "200G" {
		t.Errorf("Size = %q, want %q", vol.Size, "200G")
	}
}

func TestParseNetworkVolume(t *testing.T) {
	tmpDir := t.TempDir()
	volFile := filepath.Join(tmpDir, "shared")
	content := "mount: /mnt/shared\nsource: 192.168.1.10:/export/data\ntype: nfs\noptions: rw,soft\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "shared", "90-nfs")
	if err != nil {
		t.Fatalf("parseVolumeFile: %v", err)
	}

	if vol.Kind != VolumeNetwork {
		t.Errorf("Kind = %v, want VolumeNetwork", vol.Kind)
	}
	if vol.Source != "192.168.1.10:/export/data" {
		t.Errorf("Source = %q, want %q", vol.Source, "192.168.1.10:/export/data")
	}
	if vol.Type != "nfs" {
		t.Errorf("Type = %q, want %q", vol.Type, "nfs")
	}
	if vol.Options != "rw,soft" {
		t.Errorf("Options = %q, want %q", vol.Options, "rw,soft")
	}
}

func TestParseVolumeFileComments(t *testing.T) {
	tmpDir := t.TempDir()
	volFile := filepath.Join(tmpDir, "data")
	content := "# This is a comment\nmount: /data\n\n# Another comment\nsize: 10G\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "data", "10-base")
	if err != nil {
		t.Fatalf("parseVolumeFile: %v", err)
	}

	if vol.Kind != VolumeBlock {
		t.Errorf("Kind = %v, want VolumeBlock", vol.Kind)
	}
	if vol.Mount != "/data" {
		t.Errorf("Mount = %q, want %q", vol.Mount, "/data")
	}
}

func TestParseVolumeValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		wantErr  string
	}{
		{
			name:     "block volume missing mount",
			filename: "data",
			content:  "size: 10G\n",
			wantErr:  "must have mount:",
		},
		{
			name:     "root volume missing size",
			filename: "root",
			content:  "mount: /\n",
			wantErr:  "must have size:",
		},
		{
			name:     "network mount missing type",
			filename: "shared",
			content:  "mount: /mnt/shared\nsource: 192.168.1.10:/export\n",
			wantErr:  "must have type:",
		},
		{
			name:     "network mount missing mount",
			filename: "shared",
			content:  "source: 192.168.1.10:/export\ntype: nfs\n",
			wantErr:  "must have mount:",
		},
		{
			name:     "no size or source",
			filename: "mystery",
			content:  "mount: /data\n",
			wantErr:  "must have either size:",
		},
		{
			name:     "block volume name too long for ext4",
			filename: "my-database-vol",
			content:  "mount: /data\nsize: 10G\n",
			wantErr:  "exceeds ext4 limit of 16 characters",
		},
		{
			name:     "block volume name too long for xfs",
			filename: "longname",
			content:  "mount: /data\nsize: 10G\nformat: xfs\n",
			wantErr:  "exceeds xfs limit of 12 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			volFile := filepath.Join(tmpDir, tt.filename)
			if err := os.WriteFile(volFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := parseVolumeFile(volFile, tt.filename, "test-layer")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseBlockVolumeExactLabelLimit(t *testing.T) {
	tmpDir := t.TempDir()
	// name "exactlength" is 11 chars, label "tank-exactlength" is 16 chars — exactly at ext4 limit
	volFile := filepath.Join(tmpDir, "exactlength")
	content := "mount: /data\nsize: 10G\n"
	if err := os.WriteFile(volFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	vol, err := parseVolumeFile(volFile, "exactlength", "test-layer")
	if err != nil {
		t.Fatalf("expected no error for label at exact limit, got: %v", err)
	}
	if vol.Kind != VolumeBlock {
		t.Errorf("Kind = %v, want VolumeBlock", vol.Kind)
	}
}

func TestCollectVolumes(t *testing.T) {
	layers := []Layer{
		{
			Name: "10-base",
			Volumes: []Volume{
				{Name: "root", Kind: VolumeRoot, Size: "80G", Layer: "10-base"},
			},
		},
		{
			Name: "50-postgres",
			Volumes: []Volume{
				{Name: "pgdata", Kind: VolumeBlock, Mount: "/var/lib/postgresql", Size: "20G", Layer: "50-postgres"},
			},
		},
		{
			Name: "70-nfs",
			Volumes: []Volume{
				{Name: "shared", Kind: VolumeNetwork, Mount: "/mnt/shared", Source: "10.0.0.1:/data", Type: "nfs", Layer: "70-nfs"},
			},
		},
	}

	blocks, networks, rootSize, err := CollectVolumes(layers)
	if err != nil {
		t.Fatalf("CollectVolumes: %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("got %d block volumes, want 1", len(blocks))
	}
	if blocks[0].Name != "pgdata" {
		t.Errorf("block[0].Name = %q, want %q", blocks[0].Name, "pgdata")
	}

	if len(networks) != 1 {
		t.Fatalf("got %d network mounts, want 1", len(networks))
	}
	if networks[0].Name != "shared" {
		t.Errorf("network[0].Name = %q, want %q", networks[0].Name, "shared")
	}

	if rootSize != "80G" {
		t.Errorf("rootSize = %q, want %q", rootSize, "80G")
	}
}

func TestCollectVolumesRootMax(t *testing.T) {
	layers := []Layer{
		{
			Name: "10-base",
			Volumes: []Volume{
				{Name: "root", Kind: VolumeRoot, Size: "80G", Layer: "10-base"},
			},
		},
		{
			Name: "50-ml",
			Volumes: []Volume{
				{Name: "root", Kind: VolumeRoot, Size: "200G", Layer: "50-ml"},
			},
		},
	}

	_, _, rootSize, err := CollectVolumes(layers)
	if err != nil {
		t.Fatalf("CollectVolumes: %v", err)
	}

	if rootSize != "200G" {
		t.Errorf("rootSize = %q, want %q (largest)", rootSize, "200G")
	}
}

func TestCollectVolumesDuplicateName(t *testing.T) {
	layers := []Layer{
		{
			Name: "50-postgres",
			Volumes: []Volume{
				{Name: "data", Kind: VolumeBlock, Mount: "/var/lib/postgresql", Size: "20G", Layer: "50-postgres"},
			},
		},
		{
			Name: "60-other",
			Volumes: []Volume{
				{Name: "data", Kind: VolumeBlock, Mount: "/data", Size: "10G", Layer: "60-other"},
			},
		},
	}

	_, _, _, err := CollectVolumes(layers)
	if err == nil {
		t.Fatal("expected error for duplicate volume name, got nil")
	}
	if !strings.Contains(err.Error(), "declared in both") {
		t.Errorf("error = %q, want substring %q", err.Error(), "declared in both")
	}
}

func TestParseSizeBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"20G", 20 * 1024 * 1024 * 1024},
		{"200G", 200 * 1024 * 1024 * 1024},
		{"512M", 512 * 1024 * 1024},
		{"1T", 1024 * 1024 * 1024 * 1024},
		{"100K", 100 * 1024},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSizeBytes(tt.input)
			if got != tt.want {
				t.Errorf("parseSizeBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadLayerVolumes(t *testing.T) {
	tmpDir := t.TempDir()
	volumesDir := filepath.Join(tmpDir, "volumes")
	if err := os.MkdirAll(volumesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two volume files
	if err := os.WriteFile(filepath.Join(volumesDir, "pgdata"), []byte("mount: /var/lib/postgresql\nsize: 20G\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volumesDir, "root"), []byte("size: 80G\n"), 0644); err != nil {
		t.Fatal(err)
	}

	vols, err := loadLayerVolumes(tmpDir, "50-postgres")
	if err != nil {
		t.Fatalf("loadLayerVolumes: %v", err)
	}

	if len(vols) != 2 {
		t.Fatalf("got %d volumes, want 2", len(vols))
	}
}

func TestLoadLayerVolumesNoDir(t *testing.T) {
	tmpDir := t.TempDir()

	vols, err := loadLayerVolumes(tmpDir, "10-base")
	if err != nil {
		t.Fatalf("loadLayerVolumes: %v", err)
	}

	if len(vols) != 0 {
		t.Errorf("got %d volumes, want 0", len(vols))
	}
}
