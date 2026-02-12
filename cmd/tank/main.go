package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rhettg/tank/build"
	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/share"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

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

func ipWaitStageInfo(inst *instance.Instance, stage int) []string {
	var details []string

	stateCmd := exec.Command("virsh", "-c", "qemu:///system", "domstate", inst.Domain)
	if output, err := stateCmd.CombinedOutput(); err == nil {
		state := strings.TrimSpace(string(output))
		details = append(details, fmt.Sprintf("- VM state: %s", state))
	} else {
		details = append(details, "- VM state: unknown (could not query)")
	}

	if output, err := exec.Command("ip", "link", "show", "virbr0").CombinedOutput(); err == nil {
		if strings.Contains(string(output), "state UP") {
			details = append(details, "- virbr0: up")
		} else {
			details = append(details, "- virbr0: down")
		}
	} else {
		details = append(details, "- virbr0: unknown (could not inspect)")
	}

	if stage >= 2 {
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
			details = append(details, "- libvirt default network: unknown (could not query)")
		}
	}

	if stage >= 3 {
		leaseCmd := exec.Command("virsh", "-c", "qemu:///system", "net-dhcp-leases", "default")
		if output, err := leaseCmd.CombinedOutput(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) <= 1 {
				details = append(details, "- DHCP leases: none")
			} else {
				details = append(details, fmt.Sprintf("- DHCP leases: %d", len(lines)-1))
			}
		} else {
			details = append(details, "- DHCP leases: unknown (could not query)")
		}
	}

	return details
}

// ensureRunning makes sure the named instance is built, created, and running.
func ensureRunning(projectPath string, instanceName string, cpus int, memory int) error {
	if errs := build.Preflight(); build.PrintPreflightErrors(errs) {
		return fmt.Errorf("preflight checks failed")
	}

	p, err := project.Load(projectPath)
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	if p.CloudInit == "" {
		return fmt.Errorf("no cloud-init.yaml found in project directory (run 'tank init' to create one)")
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
	buildImagePath, err := build.Build(p, os.Stdout)
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

func main() {
	rootCmd := &cobra.Command{
		Use:   "tank",
		Short: ui.Title.Render("Tank") + " — deterministic VM images",
		Long:  "Build and run virtual machines using libvirt and KVM.",
	}

	var projectPath string
	rootCmd.PersistentFlags().StringVarP(&projectPath, "project", "p", ".", "path to project directory")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", ui.Bold.Render("tank"), ui.Highlight.Render(getVersion()))
		},
	}

	initCmd := &cobra.Command{
		Use:   "init <base-url>",
		Short: "Initialize a new tank project",
		Long:  "Create a new project directory with BASE, cloud-init.yaml, and a starter layer.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL := args[0]
			dir := projectPath

			// Don't overwrite existing project
			if _, err := os.Stat(filepath.Join(dir, "BASE")); err == nil {
				return fmt.Errorf("project already exists in %s (BASE file found)", dir)
			}

			// Create BASE file
			if err := os.WriteFile(filepath.Join(dir, "BASE"), []byte(baseURL+"\n"), 0644); err != nil {
				return fmt.Errorf("writing BASE: %w", err)
			}

			// Generate and write cloud-init.yaml
			cloudInit, err := instance.DefaultCloudInit()
			if err != nil {
				return fmt.Errorf("generating cloud-init: %w", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "cloud-init.yaml"), []byte(cloudInit), 0644); err != nil {
				return fmt.Errorf("writing cloud-init.yaml: %w", err)
			}

			// Symlink shared user-ssh layer for SSH key injection at start time
			userSSHTarget, err := share.LayerPath("00-user-ssh")
			if err != nil {
				return fmt.Errorf("finding shared user-ssh layer: %w", err)
			}
			layersDir := filepath.Join(dir, "layers")
			if err := os.MkdirAll(layersDir, 0755); err != nil {
				return fmt.Errorf("creating layers directory: %w", err)
			}
			userSSHLink := filepath.Join(layersDir, "20-user-ssh")
			os.Remove(userSSHLink) // remove stale symlink if present
			if err := os.Symlink(userSSHTarget, userSSHLink); err != nil {
				return fmt.Errorf("symlinking user-ssh layer: %w", err)
			}

			// Create starter layer
			layerDir := filepath.Join(layersDir, "10-base")
			if err := os.MkdirAll(layerDir, 0755); err != nil {
				return fmt.Errorf("creating layer directory: %w", err)
			}
			installScript := "#!/bin/bash\nset -e\n\n# Add your base provisioning here\n"
			if err := os.WriteFile(filepath.Join(layerDir, "install"), []byte(installScript), 0755); err != nil {
				return fmt.Errorf("writing install: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Initialized project in %s", dir)
			ui.PrintStep(os.Stdout, "BASE: %s", baseURL)
			ui.PrintStep(os.Stdout, "cloud-init.yaml: generated with current user")
			ui.PrintStep(os.Stdout, "layers/20-user-ssh → %s", userSSHTarget)
			ui.PrintStep(os.Stdout, "layers/10-base/install: starter script")
			return nil
		},
	}

	layersCmd := &cobra.Command{
		Use:   "layers",
		Short: "List project layers and their content hashes",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			rows := make([]ui.LayerRow, len(p.Layers))
			for i, layer := range p.Layers {
				rows[i] = ui.LayerRow{
					Name:   layer.Name,
					Script: layer.HasScript,
					Files:  layer.HasFiles,
					Hash:   layer.ContentHash,
				}
			}
			fmt.Println(ui.RenderLayerTable(p.Base, rows))
			return nil
		},
	}

	var dryRun bool
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build a VM image from project layers",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			if dryRun {
				return build.PrintPlan(os.Stdout, p)
			}

			if errs := build.Preflight(); build.PrintPreflightErrors(errs) {
				return fmt.Errorf("preflight checks failed")
			}

			buildImagePath, err := build.Build(p, os.Stdout)
			if err != nil {
				return fmt.Errorf("build: %w", err)
			}
			fmt.Printf("Build image ready: %s\n", buildImagePath)

			return nil
		},
	}
	buildCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show build plan without executing")

	var startCPUs int
	var startMemory int
	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start the VM (builds image if needed)",
		Long:  "Start a VM instance. Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceName := ""
			if len(args) > 0 {
				instanceName = args[0]
			}
			return ensureRunning(projectPath, instanceName, startCPUs, startMemory)
		},
	}
	startCmd.Flags().IntVar(&startCPUs, "cpus", 2, "number of CPUs")
	startCmd.Flags().IntVar(&startMemory, "memory", 4096, "memory in MB")

	stopCmd := &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop the VM",
		Long:  "Stop a VM instance (graceful shutdown). Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine instance name
			instanceName, err := resolveInstanceName(projectPath, args)
			if err != nil {
				return err
			}

			inst, err := instance.Load(instanceName)
			if err != nil {
				return fmt.Errorf("loading instance: %w", err)
			}

			if !inst.IsRunning() {
				ui.PrintInfo(os.Stdout, "Instance %s is not running", ui.Bold.Render(instanceName))
				return nil
			}

			if err := inst.Stop(os.Stdout); err != nil {
				return fmt.Errorf("stopping VM: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Instance %s stopping", ui.Bold.Render(instanceName))
			return nil
		},
	}

	destroyCmd := &cobra.Command{
		Use:   "destroy [name]",
		Short: "Stop and remove the VM completely",
		Long:  "Destroy a VM instance (force stop and remove all files). Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine instance name
			instanceName, err := resolveInstanceName(projectPath, args)
			if err != nil {
				return err
			}

			inst, err := instance.Load(instanceName)
			if err != nil {
				return fmt.Errorf("loading instance: %w", err)
			}

			ui.PrintInfo(os.Stdout, "Destroying instance %s", ui.Bold.Render(instanceName))
			if err := inst.Destroy(os.Stdout); err != nil {
				return fmt.Errorf("destroying instance: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Instance %s destroyed", ui.Bold.Render(instanceName))
			return nil
		},
	}

	sshCmd := &cobra.Command{
		Use:   "ssh [name] [-- ssh_args...]",
		Short: "SSH into a running VM (auto-starts if needed)",
		Long:  "Connect to a VM instance via SSH, building and starting it if necessary. Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Split args at -- into instance name args and SSH args
			var nameArgs, extraSSHArgs []string
			dashIdx := cmd.ArgsLenAtDash()
			if dashIdx >= 0 {
				nameArgs = args[:dashIdx]
				extraSSHArgs = args[dashIdx:]
			} else {
				nameArgs = args
			}

			// Determine instance name
			instanceName, err := resolveInstanceName(projectPath, nameArgs)
			if err != nil {
				return err
			}

			// Ensure instance is running (build/create/start as needed)
			if err := ensureRunning(projectPath, instanceName, 2, 4096); err != nil {
				return err
			}

			inst, err := instance.Load(instanceName)
			if err != nil {
				return fmt.Errorf("loading instance: %w", err)
			}

			// Get current username (matches cloud-init user)
			currentUser, err := user.Current()
			if err != nil {
				return fmt.Errorf("getting current user: %w", err)
			}

			// Get VM IP address with retries
			var ip string
			stageShown := map[int]bool{}
			for attempt := 0; attempt < 30; attempt++ {
				ip, err = inst.IPAddress()
				if err != nil {
					return err
				}
				if ip != "" {
					break
				}
				if attempt == 0 {
					fmt.Fprintf(os.Stderr, "Waiting for VM to get an IP address...")
				} else {
					fmt.Fprintf(os.Stderr, ".")
				}

				if attempt == 5 && !stageShown[1] {
					stageShown[1] = true
					fmt.Fprintln(os.Stderr)
					ui.PrintInfo(os.Stderr, "Still waiting for DHCP. Quick check:")
					for _, line := range ipWaitStageInfo(inst, 1) {
						fmt.Fprintln(os.Stderr, "  "+line)
					}
					fmt.Fprint(os.Stderr, "Continuing to wait...")
				}

				if attempt == 10 && !stageShown[2] {
					stageShown[2] = true
					fmt.Fprintln(os.Stderr)
					ui.PrintInfo(os.Stderr, "Still waiting. Network status:")
					for _, line := range ipWaitStageInfo(inst, 2) {
						fmt.Fprintln(os.Stderr, "  "+line)
					}
					fmt.Fprint(os.Stderr, "Continuing to wait...")
				}

				if attempt == 20 && !stageShown[3] {
					stageShown[3] = true
					fmt.Fprintln(os.Stderr)
					ui.PrintInfo(os.Stderr, "Still waiting. DHCP lease status:")
					for _, line := range ipWaitStageInfo(inst, 3) {
						fmt.Fprintln(os.Stderr, "  "+line)
					}
					fmt.Fprint(os.Stderr, "Continuing to wait...")
				}

				time.Sleep(2 * time.Second)
			}
			if ip == "" {
				diagnosis := diagnoseIPTimeout(inst)
				return fmt.Errorf("timed out waiting for VM IP address.%s", diagnosis)
			}

			// Build SSH command
			sshArgs := []string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
				fmt.Sprintf("%s@%s", currentUser.Username, ip),
			}

			// Append any extra args passed after --
			sshArgs = append(sshArgs, extraSSHArgs...)

			// Wait for SSH to become available by probing quietly
			sshReady := false
			for attempt := 0; attempt < 30; attempt++ {
				probe := exec.Command("ssh", append(sshArgs, "true")...)
				if output, err := probe.CombinedOutput(); err != nil {
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) && exitErr.ExitCode() == 255 {
						if attempt == 0 {
							fmt.Fprintf(os.Stderr, "Waiting for SSH to become available...")
						} else {
							fmt.Fprintf(os.Stderr, ".")
						}
						time.Sleep(2 * time.Second)
						continue
					}
					// Non-connection error (e.g. auth failure)
					fmt.Fprintf(os.Stderr, "%s", output)
					return fmt.Errorf("SSH failed: %w", err)
				}
				// Probe succeeded — SSH is ready
				if attempt > 0 {
					fmt.Fprintf(os.Stderr, "\n")
				}
				sshReady = true
				break
			}
			if !sshReady {
				fmt.Fprintf(os.Stderr, "\n")
				return fmt.Errorf("timed out waiting for SSH connection")
			}

			// Run the real interactive SSH session
			sshExec := exec.Command("ssh", sshArgs...)
			sshExec.Stdin = os.Stdin
			sshExec.Stdout = os.Stdout
			sshExec.Stderr = os.Stderr

			if err := sshExec.Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					os.Exit(exitErr.ExitCode())
				}
				return err
			}
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list", "ps"},
		Short:   "List all VM instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheDir, err := build.CacheDir()
			if err != nil {
				return err
			}

			instancesDir := filepath.Join(cacheDir, "instances")
			entries, err := os.ReadDir(instancesDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println(ui.MutedStyle.Render("No instances found"))
					return nil
				}
				return err
			}

			var rows []ui.InstanceRow
			for _, entry := range entries {
				name := entry.Name()
				entryPath := filepath.Join(instancesDir, name)

				// Use os.Stat to follow symlinks
				info, err := os.Stat(entryPath)
				if err != nil || !info.IsDir() {
					continue
				}

				inst, err := instance.Load(name)
				if err != nil {
					continue
				}

				status := ui.FormatStatus(inst.IsRunning())

				ip := "-"
				if inst.IsRunning() {
					if addr, err := inst.IPAddress(); err == nil && addr != "" {
						ip = addr
					}
				}

				rows = append(rows, ui.InstanceRow{
					Name:   name,
					Status: status,
					IP:     ip,
				})
			}

			if len(rows) == 0 {
				fmt.Println(ui.MutedStyle.Render("No instances found"))
				return nil
			}

			fmt.Println(ui.RenderInstanceTable(rows))
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show project status",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(projectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			instanceName, err := resolveInstanceName(projectPath, args)
			if err != nil {
				return err
			}

			projectHash := p.Hash()
			buildCached := build.BuildImageExists(projectHash)
			baseCached := build.BaseImageCached(p.Base)

			instExists := instance.Exists(instanceName)
			var inst *instance.Instance
			if instExists {
				inst, err = instance.Load(instanceName)
				if err != nil {
					return fmt.Errorf("loading instance: %w", err)
				}
			}

			state := ui.MutedStyle.Render("not created")
			ip := "-"
			if instExists {
				running := inst.IsRunning()
				state = ui.FormatStatus(running)
				if running {
					if addr, err := inst.IPAddress(); err == nil && addr != "" {
						ip = addr
					}
				}
			}

			cacheStatus := ui.WarningStyle.Render("missing")
			if buildCached {
				cacheStatus = ui.SuccessStyle.Render("cached")
			}

			instanceImageStatus := ui.MutedStyle.Render("none")
			if instExists {
				backingPath, err := backingFilePath(inst.DiskPath)
				if err != nil {
					instanceImageStatus = ui.MutedStyle.Render("unknown")
				} else {
					backingBase := strings.TrimSuffix(filepath.Base(backingPath), ".qcow2")
					backingShort := backingBase
					if len(backingShort) > 8 {
						backingShort = backingShort[:8]
					}
					if backingBase == projectHash {
						instanceImageStatus = ui.SuccessStyle.Render("fresh")
					} else {
						instanceImageStatus = ui.WarningStyle.Render(fmt.Sprintf("stale (built %s)", backingShort))
					}
				}
			}

			baseStatus := ui.WarningStyle.Render("missing")
			if baseCached {
				baseStatus = ui.SuccessStyle.Render("cached")
			}

			fmt.Printf("%s %s\n", ui.Bold.Render("Project:"), ui.MutedStyle.Render(p.Root))
			fmt.Printf("%s %s %s\n", ui.Bold.Render("Base:"), ui.Highlight.Render(p.Base), baseStatus)
			fmt.Printf("%s %s\n", ui.Bold.Render("Instance:"), ui.Highlight.Render(instanceName))
			fmt.Printf("%s %s\n", ui.Bold.Render("State:"), state)
			fmt.Printf("%s %s\n", ui.Bold.Render("IP:"), ip)
			fmt.Printf("%s %s %s\n", ui.Bold.Render("Build:"), ui.FormatHash(projectHash), cacheStatus)
			fmt.Printf("%s %s\n", ui.Bold.Render("Instance image:"), instanceImageStatus)
			fmt.Println()

			fmt.Printf("%s\n", ui.Bold.Render("Layers:"))
			if len(p.Layers) == 0 {
				fmt.Println(ui.MutedStyle.Render("No layers"))
			} else {
				for _, layer := range p.Layers {
					flags := []string{}
					if layer.HasScript {
						flags = append(flags, "install")
					}
					if layer.HasFiles {
						flags = append(flags, "files")
					}
					if layer.HasFirstboot {
						flags = append(flags, "firstboot")
					}
					if layer.HasPreboot {
						flags = append(flags, "preboot")
					}
					flagStr := ""
					if len(flags) > 0 {
						flagStr = " (" + strings.Join(flags, ", ") + ")"
					}
					fmt.Printf("  %s %s%s\n", ui.SymbolDot, ui.Bold.Render(layer.Name), flagStr)
					fmt.Printf("    %s\n", ui.MutedStyle.Render(layer.ContentHash[:8]))
				}
			}
			fmt.Println()

			blocks, networks, rootSize, err := project.CollectVolumes(p.Layers)
			if err != nil {
				return fmt.Errorf("collecting volumes: %w", err)
			}

			fmt.Printf("%s\n", ui.Bold.Render("Volumes:"))
			if len(blocks) == 0 && len(networks) == 0 && rootSize == "" {
				fmt.Println(ui.MutedStyle.Render("No volumes"))
				return nil
			}
			if rootSize != "" {
				fmt.Printf("  %s root %s\n", ui.SymbolDot, rootSize)
			}
			for _, vol := range blocks {
				owner := ""
				if vol.Owner != "" {
					owner = fmt.Sprintf(" owner:%s", vol.Owner)
				}
				fmt.Printf("  %s %s block %s → %s (%s%s)\n",
					ui.SymbolDot,
					ui.Bold.Render(vol.Name),
					vol.Size,
					vol.Mount,
					vol.Format,
					owner,
				)
			}
			for _, vol := range networks {
				opts := ""
				if vol.Options != "" {
					opts = " " + vol.Options
				}
				fmt.Printf("  %s %s %s %s → %s%s\n",
					ui.SymbolDot,
					ui.Bold.Render(vol.Name),
					vol.Type,
					vol.Source,
					vol.Mount,
					opts,
				)
			}
			return nil
		},
	}

	makeVolumeListCmd := func(use string, aliases []string, volumeListAll *bool) *cobra.Command {
		return &cobra.Command{
			Use:     use,
			Aliases: aliases,
			Short:   "List volumes",
			Long:    "List persistent volumes. By default shows volumes for the current project's instances.",
			RunE: func(cmd *cobra.Command, args []string) error {
				var filter string
				if !*volumeListAll {
					// Use current project's directory name as instance filter
					absPath, err := filepath.Abs(".")
					if err != nil {
						return err
					}
					filter = filepath.Base(absPath)
				}

				volumes, err := instance.ListVolumes(filter)
				if err != nil {
					return fmt.Errorf("listing volumes: %w", err)
				}

				var rows []ui.VolumeRow
				for _, vol := range volumes {
					// Format size
					sizeStr := formatVolumeSize(vol.Size)
					// Check if instance exists
					instanceStatus := vol.InstanceName
					if !instance.Exists(vol.InstanceName) {
						instanceStatus = vol.InstanceName + " " + ui.MutedStyle.Render("(orphaned)")
					}

					rows = append(rows, ui.VolumeRow{
						Name:     vol.Name,
						Size:     sizeStr,
						Instance: instanceStatus,
						Path:     vol.Path,
					})
				}

				if len(rows) == 0 {
					if *volumeListAll {
						fmt.Println(ui.MutedStyle.Render("No volumes found"))
					} else {
						fmt.Println(ui.MutedStyle.Render("No volumes found for this project"))
					}
					return nil
				}

				fmt.Println(ui.RenderVolumeTable(rows))
				return nil
			},
		}
	}

	// Volume management commands
	volumeCmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage persistent volumes",
	}

	var volumeListAll bool
	volumeListCmd := makeVolumeListCmd("ls", []string{"list"}, &volumeListAll)
	volumeListCmd.Flags().BoolVar(&volumeListAll, "all", false, "list all volumes including orphaned")

	var volumeRmForce bool
	volumeRmCmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a volume",
		Long:  "Remove a persistent volume by its full name (e.g. myproject-pgdata). Requires confirmation.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fullName := args[0]

			// Confirm deletion
			fmt.Printf("Remove volume %s? [y/N] ", ui.Bold.Render(fullName))
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Cancelled")
				return nil
			}

			if err := instance.RemoveVolume(fullName, volumeRmForce); err != nil {
				return fmt.Errorf("removing volume: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Volume %s removed", ui.Bold.Render(fullName))
			return nil
		},
	}
	volumeRmCmd.Flags().BoolVar(&volumeRmForce, "force", false, "force removal even if attached to a running instance")

	volumeCmd.AddCommand(volumeListCmd)
	volumeCmd.AddCommand(volumeRmCmd)

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(layersCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(volumeCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
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

	if revision != "" {
		version = revision[:min(7, len(revision))]
		if dirty {
			version += "-dirty"
		}
	}

	return version
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
