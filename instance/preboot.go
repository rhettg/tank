package instance

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rhettg/graystone/project"
	"github.com/rhettg/graystone/ui"
)

// RunPrebootHooks executes preboot hooks for all layers that have them.
// The cloudInitPath is a writable file that hooks can edit in place.
// Returns the final cloud-init content after all hooks have run.
func RunPrebootHooks(p *project.Project, instanceName, cloudInitPath string, progress io.Writer) error {
	for _, layer := range p.Layers {
		if !layer.HasPreboot {
			continue
		}

		if err := runPrebootHook(p, layer, instanceName, cloudInitPath, progress); err != nil {
			return fmt.Errorf("preboot hook for layer %s: %w", layer.Name, err)
		}
	}
	return nil
}

func runPrebootHook(p *project.Project, layer project.Layer, instanceName, cloudInitPath string, progress io.Writer) error {
	ui.PrintStep(progress, "Running preboot hook: %s", ui.Bold.Render(layer.Name))

	prebootPath := filepath.Join(layer.Path, "preboot")

	// Create a temporary work directory for the hook
	workDir, err := os.MkdirTemp("", fmt.Sprintf("gi-preboot-%s-", layer.Name))
	if err != nil {
		return fmt.Errorf("creating work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	cmd := exec.Command(prebootPath)
	cmd.Dir = layer.Path
	cmd.Env = append(os.Environ(),
		"GI_PROJECT_ROOT="+p.Root,
		"GI_INSTANCE_NAME="+instanceName,
		"GI_LAYER_PATH="+layer.Path,
		"GI_CLOUD_INIT="+cloudInitPath,
		"GI_WORK_DIR="+workDir,
	)
	cmd.Stdout = progress
	cmd.Stderr = progress

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook failed: %w", err)
	}

	return nil
}
