package build

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

// ImageFormat returns the format of a disk image (e.g., "qcow2", "raw")
// by running qemu-img info and parsing the JSON output.
func ImageFormat(path string) (string, error) {
	out, err := exec.Command("qemu-img", "info", "--output=json", path).Output()
	if err != nil {
		return "", fmt.Errorf("qemu-img info %s: %w", path, err)
	}

	var info struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("parsing qemu-img info output: %w", err)
	}

	return info.Format, nil
}

// CreateOverlay creates a qcow2 overlay image backed by backingPath.
func CreateOverlay(overlayPath, backingPath string) error {
	absBackingPath, err := filepath.Abs(backingPath)
	if err != nil {
		return fmt.Errorf("resolving backing path: %w", err)
	}

	backingFmt, err := ImageFormat(absBackingPath)
	if err != nil {
		return err
	}

	cmd := exec.Command("qemu-img", "create",
		"-f", "qcow2",
		"-F", backingFmt,
		"-b", absBackingPath,
		overlayPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img create overlay: %s: %w", out, err)
	}

	return nil
}
