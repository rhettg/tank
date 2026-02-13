package instance

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rhettg/tank/project"
	"github.com/rhettg/tank/ui"
)

// ValidateCloudInit checks that cloud-init content is well-formed.
// It ensures the #cloud-config header is present (prepending it if missing)
// and that the body looks like valid YAML key-value pairs.
func ValidateCloudInit(content string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("cloud-init content is empty")
	}

	body := content
	hasHeader := false
	if strings.HasPrefix(content, "#cloud-config") {
		hasHeader = true
		if idx := strings.Index(content, "\n"); idx >= 0 {
			body = content[idx+1:]
		} else {
			body = ""
		}
	}

	// Check that the body has at least one top-level key (word followed by colon)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(trimmed, ":") {
			return "", fmt.Errorf("cloud-init body does not look like valid YAML (line %q has no key: value)", trimmed)
		}
		// First non-comment, non-empty line has a colon — looks like a mapping
		break
	}

	if !hasHeader {
		content = "#cloud-config\n" + content
	}

	return content, nil
}

// RunPrebootHooks executes preboot hooks for all layers that have them.
// The cloudInitPath is a writable file that hooks can edit in place.
// Returns the final cloud-init content after all hooks have run.
func RunPrebootHooks(p *project.Project, instanceName, cloudInitPath string, progress io.Writer) error {
	// Load .env file from project root
	envVars, err := project.LoadEnvFile(p.Root)
	if err != nil {
		return fmt.Errorf("loading .env file: %w", err)
	}

	for _, layer := range p.Layers {
		if !layer.HasPreboot {
			continue
		}

		if err := runPrebootHook(p, layer, instanceName, cloudInitPath, envVars, progress); err != nil {
			return fmt.Errorf("preboot hook for layer %s: %w", layer.Name, err)
		}
	}
	return nil
}

func runPrebootHook(p *project.Project, layer project.Layer, instanceName, cloudInitPath string, envVars []string, progress io.Writer) error {
	ui.PrintStep(progress, "Running preboot hook: %s", ui.Bold.Render(layer.Name))

	prebootPath := filepath.Join(layer.Path, "preboot")

	// Create a temporary work directory for the hook
	workDir, err := os.MkdirTemp("", fmt.Sprintf("tank-preboot-%s-", layer.Name))
	if err != nil {
		return fmt.Errorf("creating work directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	cmd := exec.Command(prebootPath)
	cmd.Dir = layer.Path

	// Start with current environment, add .env vars, then TANK_* vars
	// Later values override earlier ones
	cmd.Env = append(os.Environ(), envVars...)
	cmd.Env = append(cmd.Env,
		"TANK_PROJECT_ROOT="+p.Root,
		"TANK_INSTANCE_NAME="+instanceName,
		"TANK_LAYER_PATH="+layer.Path,
		"TANK_CLOUD_INIT="+cloudInitPath,
		"TANK_WORK_DIR="+workDir,
	)
	cmd.Stdout = progress
	cmd.Stderr = progress

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook failed: %w", err)
	}

	return nil
}
