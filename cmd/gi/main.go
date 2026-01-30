package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/rhettg/graystone/build"
	"github.com/rhettg/graystone/instance"
	"github.com/rhettg/graystone/project"
	"github.com/rhettg/graystone/ui"
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

// ensureRunning makes sure the named instance is built, created, and running.
func ensureRunning(projectPath string, instanceName string, cpus int, memory int) error {
	p, err := project.Load(projectPath)
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	if p.CloudInit == "" {
		return fmt.Errorf("no cloud-init.yaml found in project directory (run 'gi init' to create one)")
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
		cloudInitFile, err := os.CreateTemp("", "gi-cloud-init-*.yaml")
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

	// Create instance
	ui.PrintInfo(os.Stdout, "Creating instance %s", ui.Bold.Render(instanceName))
	inst, err := instance.Create(instanceName, buildImagePath, cloudInitContent, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	// Start VM
	if err := inst.Start(cpus, memory, os.Stdout); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	fmt.Println()
	ui.PrintSuccess(os.Stdout, "Instance %s started", ui.Bold.Render(instanceName))
	return nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "gi",
		Short: ui.Title.Render("Graystone Industries") + " — deterministic VM images",
		Long:  "Build and run disposable virtual machines using libvirt and KVM.",
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", ui.Bold.Render("gi"), ui.Highlight.Render(getVersion()))
		},
	}

	var initProjectPath string
	initCmd := &cobra.Command{
		Use:   "init <base-url>",
		Short: "Initialize a new graystone project",
		Long:  "Create a new project directory with BASE, cloud-init.yaml, and a starter layer.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL := args[0]
			dir := initProjectPath

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

			// Create starter layer
			layerDir := filepath.Join(dir, "layers", "10-base")
			if err := os.MkdirAll(layerDir, 0755); err != nil {
				return fmt.Errorf("creating layer directory: %w", err)
			}
			installScript := "#!/bin/bash\nset -e\n\n# Add your base provisioning here\n"
			if err := os.WriteFile(filepath.Join(layerDir, "install.sh"), []byte(installScript), 0755); err != nil {
				return fmt.Errorf("writing install.sh: %w", err)
			}

			ui.PrintSuccess(os.Stdout, "Initialized project in %s", dir)
			ui.PrintStep(os.Stdout, "BASE: %s", baseURL)
			ui.PrintStep(os.Stdout, "cloud-init.yaml: generated with current user + SSH key")
			ui.PrintStep(os.Stdout, "layers/10-base/install.sh: starter script")
			return nil
		},
	}
	initCmd.Flags().StringVarP(&initProjectPath, "project", "p", ".", "path to project directory")

	var projectPath string
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
	layersCmd.Flags().StringVarP(&projectPath, "project", "p", ".", "path to project directory")

	var buildProjectPath string
	var dryRun bool
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Build a VM image from project layers",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project.Load(buildProjectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			if dryRun {
				return build.PrintPlan(os.Stdout, p)
			}

			buildImagePath, err := build.Build(p, os.Stdout)
			if err != nil {
				return fmt.Errorf("build: %w", err)
			}
			fmt.Printf("Build image ready: %s\n", buildImagePath)

			return nil
		},
	}
	buildCmd.Flags().StringVarP(&buildProjectPath, "project", "p", ".", "path to project directory")
	buildCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show build plan without executing")

	var startProjectPath string
	var startCPUs int
	var startMemory int
	startCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Build image (if needed) and start the VM",
		Long:  "Start a VM instance. Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceName := ""
			if len(args) > 0 {
				instanceName = args[0]
			}
			return ensureRunning(startProjectPath, instanceName, startCPUs, startMemory)
		},
	}
	startCmd.Flags().StringVarP(&startProjectPath, "project", "p", ".", "path to project directory")
	startCmd.Flags().IntVar(&startCPUs, "cpus", 2, "number of CPUs")
	startCmd.Flags().IntVar(&startMemory, "memory", 4096, "memory in MB")

	var stopProjectPath string
	stopCmd := &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop the VM",
		Long:  "Stop a VM instance (graceful shutdown). Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine instance name
			instanceName, err := resolveInstanceName(stopProjectPath, args)
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
	stopCmd.Flags().StringVarP(&stopProjectPath, "project", "p", ".", "path to project directory")

	var destroyProjectPath string
	destroyCmd := &cobra.Command{
		Use:   "destroy [name]",
		Short: "Stop and remove the VM completely",
		Long:  "Destroy a VM instance (force stop and remove all files). Default name is the project directory name.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine instance name
			instanceName, err := resolveInstanceName(destroyProjectPath, args)
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
	destroyCmd.Flags().StringVarP(&destroyProjectPath, "project", "p", ".", "path to project directory")

	var sshProjectPath string
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
			instanceName, err := resolveInstanceName(sshProjectPath, nameArgs)
			if err != nil {
				return err
			}

			// Ensure instance is running (build/create/start as needed)
			if err := ensureRunning(sshProjectPath, instanceName, 2, 4096); err != nil {
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
				time.Sleep(2 * time.Second)
			}
			if ip == "" {
				return fmt.Errorf("timed out waiting for VM IP address")
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
	sshCmd.Flags().StringVarP(&sshProjectPath, "project", "p", ".", "path to project directory")

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "ps"},
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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(layersCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(listCmd)

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
