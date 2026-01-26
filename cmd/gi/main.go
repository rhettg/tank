package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"

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

			// Download base image
			fmt.Println("Downloading base image...")
			baseImagePath, err := build.DownloadBaseImage(p.Base, os.Stdout)
			if err != nil {
				return fmt.Errorf("downloading base image: %w", err)
			}
			fmt.Printf("Base image ready: %s\n\n", baseImagePath)

			// Create build image
			projectHash := p.Hash()
			fmt.Printf("Creating build image (project hash: %s)...\n", projectHash[:8])
			buildImagePath, err := build.CreateBuildImage(baseImagePath, projectHash, os.Stdout)
			if err != nil {
				return fmt.Errorf("creating build image: %w", err)
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
			p, err := project.Load(startProjectPath)
			if err != nil {
				return fmt.Errorf("loading project: %w", err)
			}

			// Determine instance name
			instanceName := filepath.Base(p.Root)
			if len(args) > 0 {
				instanceName = args[0]
			}

			// Check if instance already exists
			if instance.Exists(instanceName) {
				// Load and check if running
				inst, err := instance.Load(instanceName)
				if err != nil {
					return fmt.Errorf("loading instance: %w", err)
				}
				if inst.IsRunning() {
					fmt.Printf("Instance %q is already running.\n", instanceName)
					fmt.Printf("  Connect with: virsh -c qemu:///system console %s\n", inst.Domain)
					return nil
				}
				// Instance exists but not running - start it
				fmt.Printf("Starting existing instance %q...\n", instanceName)
				cmd := exec.Command("virsh", "-c", "qemu:///system", "start", inst.Domain)
				if output, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("starting VM: %w: %s", err, output)
				}
				fmt.Printf("Instance %q started.\n", instanceName)
				fmt.Printf("  Connect with: virsh -c qemu:///system console %s\n", inst.Domain)
				return nil
			}

			// Build image if needed
			fmt.Println("Building image...")
			baseImagePath, err := build.DownloadBaseImage(p.Base, os.Stdout)
			if err != nil {
				return fmt.Errorf("downloading base image: %w", err)
			}

			projectHash := p.Hash()
			buildImagePath, err := build.CreateBuildImage(baseImagePath, projectHash, os.Stdout)
			if err != nil {
				return fmt.Errorf("creating build image: %w", err)
			}
			fmt.Printf("Build ready: %s\n\n", buildImagePath)

			// Determine cloud-init config
			cloudInit := p.CloudInit
			if cloudInit == "" {
				fmt.Println("No cloud-init.yaml found, generating default (user + SSH key)...")
				cloudInit, err = instance.DefaultCloudInit()
				if err != nil {
					return fmt.Errorf("generating default cloud-init: %w", err)
				}
			}

			// Create instance
			fmt.Printf("Creating instance %q...\n", instanceName)
			inst, err := instance.Create(instanceName, buildImagePath, cloudInit, os.Stdout)
			if err != nil {
				return fmt.Errorf("creating instance: %w", err)
			}

			// Start VM
			if err := inst.Start(startCPUs, startMemory, os.Stdout); err != nil {
				return fmt.Errorf("starting VM: %w", err)
			}

			fmt.Printf("\nInstance %q started.\n", instanceName)
			fmt.Printf("  Connect with: virsh -c qemu:///system console %s\n", inst.Domain)
			return nil
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

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(layersCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(destroyCmd)

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
