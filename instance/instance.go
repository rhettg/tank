package instance

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/rhettg/graystone/build"
)

// DefaultCloudInit generates a cloud-init config that creates a user matching
// the current user with their SSH public key for passwordless access.
func DefaultCloudInit() (string, error) {
	// Get current username
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("getting current user: %w", err)
	}

	// Find SSH public key
	sshKey, err := findSSHPublicKey()
	if err != nil {
		return "", err
	}

	// Generate cloud-init YAML
	cloudInit := fmt.Sprintf(`#cloud-config
users:
  - name: %s
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - %s
`, currentUser.Username, sshKey)

	return cloudInit, nil
}

// findSSHPublicKey looks for an SSH public key in standard locations.
func findSSHPublicKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	// Try common key types in order of preference
	keyFiles := []string{
		filepath.Join(home, ".ssh", "id_ed25519.pub"),
		filepath.Join(home, ".ssh", "id_rsa.pub"),
	}

	for _, keyFile := range keyFiles {
		content, err := os.ReadFile(keyFile)
		if err == nil {
			return strings.TrimSpace(string(content)), nil
		}
	}

	return "", fmt.Errorf("no SSH public key found (tried ~/.ssh/id_ed25519.pub, ~/.ssh/id_rsa.pub)")
}

// Instance represents a running or stopped VM instance.
type Instance struct {
	Name      string // Instance name (default: project directory name)
	Dir       string // Instance directory path
	DiskPath  string // Path to COW overlay disk
	ISOPath   string // Path to cloud-init ISO
	Domain    string // libvirt domain name (gi-<name>)
}

// InstanceDir returns the directory for an instance.
func InstanceDir(name string) (string, error) {
	cacheDir, err := build.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "instances", name), nil
}

// Exists returns true if the instance already exists.
func Exists(name string) bool {
	dir, err := InstanceDir(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(dir)
	return err == nil
}

// Create creates a new instance with a COW overlay disk.
func Create(name, buildImagePath, cloudInitYAML string, progress io.Writer) (*Instance, error) {
	dir, err := InstanceDir(name)
	if err != nil {
		return nil, err
	}

	// Check if already exists
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("instance %q already exists", name)
	}

	// Create instance directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	inst := &Instance{
		Name:     name,
		Dir:      dir,
		DiskPath: filepath.Join(dir, "disk.qcow2"),
		ISOPath:  filepath.Join(dir, "cloud-init.iso"),
		Domain:   "gi-" + name,
	}

	// Create COW overlay disk backed by build image
	fmt.Fprintf(progress, "  Creating overlay disk...\n")
	if err := createOverlayDisk(inst.DiskPath, buildImagePath); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("creating overlay disk: %w", err)
	}

	// Create cloud-init ISO if we have cloud-init config
	if cloudInitYAML != "" {
		fmt.Fprintf(progress, "  Creating cloud-init ISO...\n")
		if err := createCloudInitISO(inst.ISOPath, cloudInitYAML); err != nil {
			os.RemoveAll(dir)
			return nil, fmt.Errorf("creating cloud-init ISO: %w", err)
		}
	} else {
		inst.ISOPath = ""
	}

	fmt.Fprintf(progress, "  Instance directory: %s\n", dir)
	return inst, nil
}

// createOverlayDisk creates a qcow2 overlay backed by the given image.
func createOverlayDisk(diskPath, backingFile string) error {
	// qemu-img create -f qcow2 -b <backing> -F qcow2 <new>
	cmd := exec.Command("qemu-img", "create",
		"-f", "qcow2",
		"-b", backingFile,
		"-F", "qcow2",
		diskPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

// createCloudInitISO creates a NoCloud ISO with the given user-data.
func createCloudInitISO(isoPath, userData string) error {
	// Create temp directory for ISO contents
	tmpDir, err := os.MkdirTemp("", "cloud-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Write meta-data (minimal, just instance-id)
	metaData := "instance-id: graystone\nlocal-hostname: graystone\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return err
	}

	// Write user-data
	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
		return err
	}

	// Write network-config (enable DHCP on first ethernet interface)
	networkConfig := `version: 2
ethernets:
  id0:
    match:
      driver: virtio_net
    dhcp4: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, "network-config"), []byte(networkConfig), 0644); err != nil {
		return err
	}

	// Create ISO using available tool (try genisoimage, mkisofs, then xorriso)
	var cmd *exec.Cmd
	if _, err := exec.LookPath("genisoimage"); err == nil {
		cmd = exec.Command("genisoimage",
			"-output", isoPath,
			"-volid", "cidata",
			"-joliet",
			"-rock",
			filepath.Join(tmpDir, "meta-data"),
			filepath.Join(tmpDir, "user-data"),
			filepath.Join(tmpDir, "network-config"),
		)
	} else if _, err := exec.LookPath("mkisofs"); err == nil {
		cmd = exec.Command("mkisofs",
			"-output", isoPath,
			"-volid", "cidata",
			"-joliet",
			"-rock",
			filepath.Join(tmpDir, "meta-data"),
			filepath.Join(tmpDir, "user-data"),
			filepath.Join(tmpDir, "network-config"),
		)
	} else if _, err := exec.LookPath("xorriso"); err == nil {
		cmd = exec.Command("xorriso",
			"-as", "mkisofs",
			"-output", isoPath,
			"-volid", "cidata",
			"-joliet",
			"-rock",
			filepath.Join(tmpDir, "meta-data"),
			filepath.Join(tmpDir, "user-data"),
			filepath.Join(tmpDir, "network-config"),
		)
	} else {
		return fmt.Errorf("no ISO creation tool found (need genisoimage, mkisofs, or xorriso)")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

// Start starts the VM using virt-install.
func (inst *Instance) Start(cpus, memoryMB int, progress io.Writer) error {
	fmt.Fprintf(progress, "  Starting VM %s...\n", inst.Domain)

	args := []string{
		"--connect", "qemu:///system",
		"--name", inst.Domain,
		"--memory", fmt.Sprintf("%d", memoryMB),
		"--vcpus", fmt.Sprintf("%d", cpus),
		"--disk", inst.DiskPath,
		"--import",
		"--os-variant", "linux2022",
		"--network", "default",
		"--graphics", "none",
		"--noautoconsole",
	}

	// Add cloud-init ISO if present
	if inst.ISOPath != "" {
		args = append(args, "--disk", inst.ISOPath+",device=cdrom")
	}

	cmd := exec.Command("virt-install", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}

	return nil
}

// Load loads an existing instance by name.
func Load(name string) (*Instance, error) {
	dir, err := InstanceDir(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("instance %q not found", name)
	}

	inst := &Instance{
		Name:     name,
		Dir:      dir,
		DiskPath: filepath.Join(dir, "disk.qcow2"),
		ISOPath:  filepath.Join(dir, "cloud-init.iso"),
		Domain:   "gi-" + name,
	}

	// Check if ISO exists
	if _, err := os.Stat(inst.ISOPath); os.IsNotExist(err) {
		inst.ISOPath = ""
	}

	return inst, nil
}

// IsRunning checks if the VM is currently running.
func (inst *Instance) IsRunning() bool {
	cmd := exec.Command("virsh", "-c", "qemu:///system", "domstate", inst.Domain)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "running"
}

// Stop stops the VM (graceful shutdown).
func (inst *Instance) Stop(progress io.Writer) error {
	fmt.Fprintf(progress, "  Stopping VM %s...\n", inst.Domain)
	cmd := exec.Command("virsh", "-c", "qemu:///system", "shutdown", inst.Domain)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

// Destroy forcefully stops and undefines the VM, then removes instance files.
func (inst *Instance) Destroy(progress io.Writer) error {
	// Force stop if running
	if inst.IsRunning() {
		fmt.Fprintf(progress, "  Force stopping VM %s...\n", inst.Domain)
		cmd := exec.Command("virsh", "-c", "qemu:///system", "destroy", inst.Domain)
		cmd.Run() // Ignore error if not running
	}

	// Undefine the domain
	fmt.Fprintf(progress, "  Removing VM definition...\n")
	cmd := exec.Command("virsh", "-c", "qemu:///system", "undefine", inst.Domain)
	cmd.Run() // Ignore error if not defined

	// Remove instance directory
	fmt.Fprintf(progress, "  Removing instance files...\n")
	if err := os.RemoveAll(inst.Dir); err != nil {
		return fmt.Errorf("removing instance directory: %w", err)
	}

	return nil
}
