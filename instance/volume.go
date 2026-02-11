package instance

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
)

var validVolumeName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// VolumesDir returns the directory where volumes are stored.
func VolumesDir() (string, error) {
	cacheDir, err := build.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "volumes"), nil
}

// VolumePath returns the path for a volume's qcow2 file.
func VolumePath(instanceName, volumeName string) (string, error) {
	dir, err := VolumesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, instanceName+"-"+volumeName+".qcow2"), nil
}

// VolumeLabel returns the filesystem label for a volume.
func VolumeLabel(volumeName string) string {
	return "tank-" + volumeName
}

// EnsureVolume creates a volume's qcow2 file if it doesn't already exist.
// Returns true if the volume was newly created (needs formatting), false if it already existed.
func EnsureVolume(instanceName string, vol project.Volume, progress io.Writer) (bool, error) {
	volPath, err := VolumePath(instanceName, vol.Name)
	if err != nil {
		return false, err
	}

	// Check if volume already exists
	if _, err := os.Stat(volPath); err == nil {
		ui.PrintStep(progress, "Reattaching volume %s (%s) → %s",
			ui.Bold.Render(vol.Name), vol.Size, vol.Mount)
		return false, nil
	}

	// Create volumes directory
	if err := os.MkdirAll(filepath.Dir(volPath), 0755); err != nil {
		return false, fmt.Errorf("creating volumes directory: %w", err)
	}

	// Create qcow2 image
	ui.PrintStep(progress, "Creating volume %s (%s) → %s",
		ui.Bold.Render(vol.Name), vol.Size, vol.Mount)

	cmd := exec.Command("qemu-img", "create",
		"-f", "qcow2",
		volPath,
		vol.Size,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("creating volume %s: %w: %s", vol.Name, err, output)
	}

	return true, nil
}

// VolumeCloudInit generates cloud-init YAML stanzas for disk_setup, fs_setup,
// and mounts for the given block volumes.
// Only newly created volumes get disk_setup and fs_setup entries (to format them).
// All block volumes get mount entries. Network mounts also get mount entries.
func VolumeCloudInit(blocks []project.Volume, networks []project.Volume, newVolumes map[string]bool) string {
	if len(blocks) == 0 && len(networks) == 0 {
		return ""
	}

	var sb strings.Builder

	// disk_setup: configure partition table for new volumes
	hasNew := false
	for _, vol := range blocks {
		if newVolumes[vol.Name] {
			hasNew = true
			break
		}
	}

	if hasNew {
		sb.WriteString("\ndisk_setup:\n")
		for i, vol := range blocks {
			if !newVolumes[vol.Name] {
				continue
			}
			// Use virtio device names: /dev/vdb, /dev/vdc, etc.
			// vda is root, so block volumes start at vdb.
			devLetter := string(rune('b' + i))
			dev := "/dev/vd" + devLetter
			sb.WriteString(fmt.Sprintf("  %s:\n", dev))
			sb.WriteString("    table_type: gpt\n")
			sb.WriteString("    layout: true\n")
			sb.WriteString("    overwrite: false\n")
		}

		sb.WriteString("\nfs_setup:\n")
		for i, vol := range blocks {
			if !newVolumes[vol.Name] {
				continue
			}
			devLetter := string(rune('b' + i))
			dev := "/dev/vd" + devLetter + "1"
			sb.WriteString(fmt.Sprintf("  - label: %s\n", VolumeLabel(vol.Name)))
			sb.WriteString(fmt.Sprintf("    filesystem: %s\n", vol.Format))
			sb.WriteString(fmt.Sprintf("    device: %s\n", dev))
			sb.WriteString("    overwrite: false\n")
		}
	}

	// mounts: all block volumes (by label) and network mounts
	if len(blocks) > 0 || len(networks) > 0 {
		sb.WriteString("\nmounts:\n")

		for _, vol := range blocks {
			sb.WriteString(fmt.Sprintf("  - [\"LABEL=%s\", \"%s\", \"%s\", \"defaults,nofail\", \"0\", \"2\"]\n",
				VolumeLabel(vol.Name), vol.Mount, vol.Format))
		}

		for _, vol := range networks {
			opts := "defaults"
			if vol.Options != "" {
				opts = vol.Options
			}
			device := vol.Source
			if vol.Type == "9p" || vol.Type == "virtiofs" {
				device = vol.Name
			}
			sb.WriteString(fmt.Sprintf("  - [\"%s\", \"%s\", \"%s\", \"%s\", \"0\", \"0\"]\n",
				device, vol.Mount, vol.Type, opts))
		}
	}

	// runcmd: chown mount points with owner: field
	var chownCmds []string
	for _, vol := range blocks {
		if vol.Owner != "" {
			chownCmds = append(chownCmds, fmt.Sprintf("  - [\"chown\", \"%s\", \"%s\"]", vol.Owner, vol.Mount))
		}
	}
	if len(chownCmds) > 0 {
		sb.WriteString("\nruncmd:\n")
		for _, cmd := range chownCmds {
			sb.WriteString(cmd + "\n")
		}
	}

	return sb.String()
}

// VolumeInfo holds information about an existing volume on disk.
type VolumeInfo struct {
	Name         string // full name: instance-volumename
	InstanceName string // instance name
	VolumeName   string // volume name (without instance prefix)
	Path         string // full path to qcow2
	Size         int64  // file size on disk
}

// knownInstances returns the set of all instance names (current and historical)
// by scanning the instances directory. This is used to correctly parse volume
// filenames when instance names contain dashes.
func knownInstances() map[string]bool {
	cacheDir, err := build.CacheDir()
	if err != nil {
		return nil
	}

	instancesDir := filepath.Join(cacheDir, "instances")
	entries, err := os.ReadDir(instancesDir)
	if err != nil {
		return nil
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() {
			names[entry.Name()] = true
		}
	}
	return names
}

// ListVolumes lists all volumes in the volumes directory.
// If instanceFilter is non-empty, only volumes for that instance are returned
// and volume names are parsed by stripping the instance prefix.
// If instanceFilter is empty, all volumes are returned. Instance names are
// resolved by matching against known instances first, then falling back to
// splitting on the last dash.
func ListVolumes(instanceFilter string) ([]VolumeInfo, error) {
	dir, err := VolumesDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// When listing all volumes, build a set of known instance names
	// to correctly parse filenames like "my-project-pgdata.qcow2".
	var instances map[string]bool
	if instanceFilter == "" {
		instances = knownInstances()
	}

	var volumes []VolumeInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".qcow2") {
			continue
		}

		fullName := strings.TrimSuffix(entry.Name(), ".qcow2")

		var instanceName, volumeName string
		if instanceFilter != "" {
			prefix := instanceFilter + "-"
			if !strings.HasPrefix(fullName, prefix) {
				continue
			}
			instanceName = instanceFilter
			volumeName = fullName[len(prefix):]
		} else {
			// Try matching against known instance names (longest match first
			// to handle nested dashes correctly).
			matched := false
			for name := range instances {
				prefix := name + "-"
				if strings.HasPrefix(fullName, prefix) {
					// Prefer longest matching instance name
					if !matched || len(name) > len(instanceName) {
						instanceName = name
						volumeName = fullName[len(prefix):]
						matched = true
					}
				}
			}
			if !matched {
				// Fallback: split on last dash (volume names are typically
				// simple identifiers without dashes).
				dashIdx := strings.LastIndex(fullName, "-")
				if dashIdx < 0 {
					continue
				}
				instanceName = fullName[:dashIdx]
				volumeName = fullName[dashIdx+1:]
			}
		}

		volPath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		volumes = append(volumes, VolumeInfo{
			Name:         fullName,
			InstanceName: instanceName,
			VolumeName:   volumeName,
			Path:         volPath,
			Size:         info.Size(),
		})
	}

	return volumes, nil
}

// RemoveVolume deletes a volume by its full name (instance-volumename).
// If force is false, it refuses to delete a volume attached to a running instance.
func RemoveVolume(fullName string, force bool) error {
	if !validVolumeName.MatchString(fullName) {
		return fmt.Errorf("invalid volume name %q: must contain only letters, digits, dots, underscores, and dashes", fullName)
	}

	dir, err := VolumesDir()
	if err != nil {
		return err
	}

	volPath := filepath.Join(dir, fullName+".qcow2")
	if _, err := os.Stat(volPath); os.IsNotExist(err) {
		return fmt.Errorf("volume %q not found", fullName)
	}

	// Check if the volume is attached to a running instance
	if !force {
		volumes, err := ListVolumes("")
		if err == nil {
			for _, vol := range volumes {
				if vol.Name == fullName {
					if Exists(vol.InstanceName) {
						inst, err := Load(vol.InstanceName)
						if err == nil && inst.IsRunning() {
							return fmt.Errorf("volume %q is attached to running instance %q; stop the instance first or use --force", fullName, vol.InstanceName)
						}
					}
					break
				}
			}
		}
	}

	return os.Remove(volPath)
}

// VolumeDisksForStart returns the virt-install --disk arguments for block volumes
// and --filesystem arguments for virtiofs/9p mounts.
func VolumeDisksForStart(instanceName string, blocks []project.Volume, networks []project.Volume) (diskArgs []string, fsArgs []string, err error) {
	for _, vol := range blocks {
		volPath, err := VolumePath(instanceName, vol.Name)
		if err != nil {
			return nil, nil, err
		}
		diskArgs = append(diskArgs, volPath)
	}

	for _, vol := range networks {
		switch vol.Type {
		case "9p":
			fsArgs = append(fsArgs, fmt.Sprintf("%s,%s,mode=mapped", vol.Source, vol.Name))
		case "virtiofs":
			fsArgs = append(fsArgs, fmt.Sprintf("%s,%s", vol.Source, vol.Name))
		}
		// NFS mounts are guest-side only (handled via cloud-init/fstab), no host args needed
	}

	return diskArgs, fsArgs, nil
}

// RetainedVolumeSummary returns a human-readable summary of volumes that
// were preserved during a destroy operation.
func RetainedVolumeSummary(instanceName string) string {
	volumes, err := ListVolumes(instanceName)
	if err != nil || len(volumes) == 0 {
		return ""
	}

	var names []string
	for _, vol := range volumes {
		names = append(names, vol.VolumeName)
	}

	return fmt.Sprintf("Persistent volumes retained: %s", strings.Join(names, ", "))
}
