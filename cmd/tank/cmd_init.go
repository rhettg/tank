package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rhettg/tank/instance"
	"github.com/rhettg/tank/share"
	"github.com/rhettg/tank/ui"
	"github.com/spf13/cobra"
)

func newInitCmd(projectPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init <base-url>",
		Short: "Initialize a new tank project",
		Long:  "Create a new project directory with BASE, cloud-init.yaml, and a starter layer.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			baseURL := args[0]
			dir := *projectPath

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
			installScript := "#!/bin/sh\nset -e\n\n# Add your base provisioning here\n"
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
}
