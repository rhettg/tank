package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
)

// buildVersion is set by release builds via -ldflags "-X main.buildVersion=vX.Y.Z".
var buildVersion string

// resolveInstanceName determines the instance name from args or project path.
func resolveInstanceName(projectPath string, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	p, err := project.Load(projectPath)
	if err != nil {
		return "", fmt.Errorf("loading project: %w", err)
	}
	return filepath.Base(p.Root), nil
}

func backingFilePath(diskPath string) (string, error) {
	cmd := exec.Command("qemu-img", "info", "-U", diskPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("qemu-img info -U: %w: %s", err, output)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "backing file:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "backing file:"))
			if idx := strings.Index(value, " ("); idx != -1 {
				value = value[:idx]
			}
			if value == "" {
				return "", fmt.Errorf("backing file not reported")
			}
			return value, nil
		}
	}

	return "", fmt.Errorf("backing file not reported")
}

func diagnoseIPTimeout(inst *instance.Instance) string {
	var details []string

	if inst.IsRunning() {
		details = append(details, "- VM is running (virsh domstate reports running)")
	} else {
		details = append(details, "- VM is not running (check boot logs via virsh console)")
	}

	netInfo := exec.Command("virsh", "-c", "qemu:///system", "net-info", "default")
	if output, err := netInfo.CombinedOutput(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			switch key {
			case "Active", "Autostart", "Bridge":
				details = append(details, fmt.Sprintf("- libvirt default network %s: %s", strings.ToLower(key), value))
			}
		}
	} else {
		details = append(details, "- could not query libvirt default network")
	}

	if output, err := exec.Command("ip", "addr", "show", "virbr0").CombinedOutput(); err == nil {
		if strings.Contains(string(output), "state UP") {
			details = append(details, "- virbr0 is up")
		} else {
			details = append(details, "- virbr0 is not up")
		}
	} else {
		details = append(details, "- could not inspect virbr0 on host")
	}

	leaseCmd := exec.Command("virsh", "-c", "qemu:///system", "net-dhcp-leases", "default")
	if output, err := leaseCmd.CombinedOutput(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) <= 1 {
			details = append(details, "- no DHCP leases reported on default network")
		} else {
			details = append(details, fmt.Sprintf("- DHCP leases found: %d", len(lines)-1))
		}
	} else {
		details = append(details, "- could not query DHCP leases")
	}

	details = append(details,
		"- If the VM is running but no lease appears, a host firewall (UFW) may be blocking DHCP/DNS or forwarded traffic.")
	details = append(details,
		"- For UFW, allow DHCP/DNS on virbr0 and routed forwarding (see docs).")

	return "\n" + strings.Join(details, "\n")
}

// ensureRunning makes sure the named instance is built, created, and running.
func ensureRunning(projectPath string, instanceName string, cpus int, memory int, noCache bool) error {
	if errs := build.Preflight(); build.PrintPreflightErrors(errs) {
		return fmt.Errorf("preflight checks failed")
	}

	p, err := project.Load(projectPath)
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	if instanceName == "" {
		instanceName = filepath.Base(p.Root)
	}

	// Check if instance already exists
	if instance.Exists(instanceName) {
		inst, err := instance.Load(instanceName)
		if err != nil {
			return fmt.Errorf("loading instance: %w", err)
		}
		if inst.IsRunning() {
			ui.PrintInfo(os.Stdout, "Instance %s is already running", ui.Bold.Render(instanceName))
			return nil
		}
		// Instance exists but not running - start it
		ui.PrintInfo(os.Stdout, "Starting existing instance %s", ui.Bold.Render(instanceName))
		cmd := exec.Command("virsh", "-c", "qemu:///system", "start", inst.Domain)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("starting VM: %w: %s", err, output)
		}
		ui.PrintSuccess(os.Stdout, "Instance %s started", ui.Bold.Render(instanceName))
		return nil
	}

	// Build image if needed
	buildImagePath, err := build.Build(p, os.Stdout, build.BuildOptions{NoCache: noCache})
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	ui.PrintStep(os.Stdout, "Build ready: %s", ui.MutedStyle.Render(buildImagePath))
	fmt.Println()

	// Prepare cloud-init content for preboot hooks
	cloudInitContent := p.CloudInit

	// Check if any layers have preboot hooks
	hasPreboot := false
	for _, layer := range p.Layers {
		if layer.HasPreboot {
			hasPreboot = true
			break
		}
	}

	if hasPreboot {
		// Write cloud-init to temp file for hooks to edit
		cloudInitFile, err := os.CreateTemp("", "tank-cloud-init-*.yaml")
		if err != nil {
			return fmt.Errorf("creating cloud-init temp file: %w", err)
		}
		cloudInitPath := cloudInitFile.Name()
		defer os.Remove(cloudInitPath)

		if _, err := cloudInitFile.WriteString(cloudInitContent); err != nil {
			cloudInitFile.Close()
			return fmt.Errorf("writing cloud-init temp file: %w", err)
		}
		cloudInitFile.Close()

		// Run preboot hooks
		if err := instance.RunPrebootHooks(p, instanceName, cloudInitPath, os.Stdout); err != nil {
			return fmt.Errorf("preboot hooks: %w", err)
		}

		// Read modified cloud-init content
		modifiedContent, err := os.ReadFile(cloudInitPath)
		if err != nil {
			return fmt.Errorf("reading modified cloud-init: %w", err)
		}
		cloudInitContent = string(modifiedContent)
	}

	// Validate cloud-init content
	if cloudInitContent != "" {
		validated, err := instance.ValidateCloudInit(cloudInitContent)
		if err != nil {
			return fmt.Errorf("invalid cloud-init after preboot hooks: %w", err)
		}
		cloudInitContent = validated
	}

	// Process volumes
	blocks, networks, rootSize, err := project.CollectVolumes(p.Layers)
	if err != nil {
		return fmt.Errorf("collecting volumes: %w", err)
	}

	// Ensure block volumes exist (create qcow2 if needed)
	newVolumes := make(map[string]bool)
	for _, vol := range blocks {
		isNew, err := instance.EnsureVolume(instanceName, vol, os.Stdout)
		if err != nil {
			return fmt.Errorf("ensuring volume %s: %w", vol.Name, err)
		}
		if isNew {
			newVolumes[vol.Name] = true
		}
	}

	// Append volume cloud-init stanzas
	cloudInitContent += instance.VolumeCloudInit(blocks, networks, newVolumes)

	// Create instance
	ui.PrintInfo(os.Stdout, "Creating instance %s", ui.Bold.Render(instanceName))
	inst, err := instance.Create(instanceName, buildImagePath, cloudInitContent, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	// Handle root disk size override
	if rootSize != "" {
		ui.PrintStep(os.Stdout, "Root disk size: %s", rootSize)
		cmd := exec.Command("qemu-img", "resize", inst.DiskPath, rootSize)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("resizing root disk: %w: %s", err, output)
		}
	}

	// Get volume disk/filesystem args for virt-install
	volumeDisks, volumeFS, err := instance.VolumeDisksForStart(instanceName, blocks, networks)
	if err != nil {
		return fmt.Errorf("preparing volume args: %w", err)
	}

	// Start VM
	if err := inst.Start(cpus, memory, volumeDisks, volumeFS, os.Stdout); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	fmt.Println()
	ui.PrintSuccess(os.Stdout, "Instance %s started", ui.Bold.Render(instanceName))
	return nil
}

func getVersion() string {
	version := "dev"
	revision := ""
	dirty := false

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
	}

	if buildVersion != "" {
		version = buildVersion
		if revision != "" {
			version += "-" + revision[:min(7, len(revision))]
		}
		if dirty {
			version += "+dirty"
		}
		return version
	}

	if revision == "" {
		return version
	}

	version = revision[:min(7, len(revision))]
	if dirty {
		version += "-dirty"
	}

	return version
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// formatVolumeSize formats a file size in bytes to a human-readable string.
func formatVolumeSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
