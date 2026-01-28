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
			fmt.Printf("Instance %q is already running.\n", instanceName)
			return nil
		}
		// Instance exists but not running - start it
		fmt.Printf("Starting existing instance %q...\n", instanceName)
		cmd := exec.Command("virsh", "-c", "qemu:///system", "start", inst.Domain)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("starting VM: %w: %s", err, output)
		}
		fmt.Printf("Instance %q started.\n", instanceName)
		return nil
	}

	// Build image if needed
	buildImagePath, err := build.Build(p, os.Stdout)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	fmt.Printf("Build ready: %s\n\n", buildImagePath)

	// Create instance
	fmt.Printf("Creating instance %q...\n", instanceName)
	inst, err := instance.Create(instanceName, buildImagePath, p.CloudInit, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	// Start VM
	if err := inst.Start(cpus, memory, os.Stdout); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	fmt.Printf("\nInstance %q started.\n", instanceName)
	return nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "gi",
		Short: "Graystone Industries - deterministic VM images",
		Long:  "Build and run disposable virtual machines using libvirt and KVM.",
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gi version %s\n", getVersion())
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

			fmt.Printf("Initialized project in %s\n", dir)
			fmt.Printf("  BASE: %s\n", baseURL)
			fmt.Println("  cloud-init.yaml: generated with current user + SSH key")
			fmt.Println("  layers/10-base/install.sh: starter script")
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

			fmt.Printf("Base: %s\n", p.Base)
			fmt.Println("Layers:")
			for _, layer := range p.Layers {
				script := "       "
				if layer.HasScript {
					script = "[script]"
				}
				files := "       "
				if layer.HasFiles {
					files = "[files]"
				}
				fmt.Printf("  %-14s %s %s  %s\n", layer.Name, script, files, layer.ContentHash[:8])
			}
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
				fmt.Printf("Instance %q is not running.\n", instanceName)
				return nil
			}

			if err := inst.Stop(os.Stdout); err != nil {
				return fmt.Errorf("stopping VM: %w", err)
			}

			fmt.Printf("Instance %q stopping.\n", instanceName)
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

			fmt.Printf("Destroying instance %q...\n", instanceName)
			if err := inst.Destroy(os.Stdout); err != nil {
				return fmt.Errorf("destroying instance: %w", err)
			}

			fmt.Printf("Instance %q destroyed.\n", instanceName)
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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(layersCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)

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
