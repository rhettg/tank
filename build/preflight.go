package build

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/rhettg/tank/ui"
)

// PreflightError represents a preflight check failure with a remediation hint.
type PreflightError struct {
	Message string
	Hint    string
}

func (e *PreflightError) Error() string {
	return e.Message
}

// Preflight runs system checks and returns all errors found.
// Returns nil if everything looks good.
func Preflight() []PreflightError {
	var errs []PreflightError

	// Check libvirt group membership
	if err := checkLibvirtGroup(); err != nil {
		errs = append(errs, *err)
	}

	// Check storage directory
	if err := checkStorageDir(); err != nil {
		errs = append(errs, *err)
	}

	// Check required tools
	toolHints := map[string]string{
		"virsh":          "Install libvirt-clients (deb) or libvirt (rpm)",
		"virt-customize": "Install libguestfs-tools (deb) or guestfs-tools (rpm)",
		"guestfish":      "Install libguestfs-tools (deb) or guestfs-tools (rpm)",
		"qemu-img":       "Install qemu-utils (deb) or qemu-img (rpm)",
		"virt-install":   "Install virtinst (deb) or virt-install (rpm)",
	}
	for _, tool := range []string{"virsh", "virt-customize", "guestfish", "qemu-img", "virt-install"} {
		if _, err := exec.LookPath(tool); err != nil {
			errs = append(errs, PreflightError{
				Message: fmt.Sprintf("%s not found in PATH", tool),
				Hint:    toolHints[tool],
			})
		}
	}

	// Check ISO creation tool
	hasISO := false
	for _, tool := range []string{"genisoimage", "mkisofs", "xorriso"} {
		if _, err := exec.LookPath(tool); err == nil {
			hasISO = true
			break
		}
	}
	if !hasISO {
		errs = append(errs, PreflightError{
			Message: "no ISO creation tool found (need genisoimage, mkisofs, or xorriso)",
			Hint:    "Install genisoimage or xorriso",
		})
	}


	// Check libvirtd is running (only if virsh is available)
	if _, err := exec.LookPath("virsh"); err == nil {
		if err := checkLibvirtd(); err != nil {
			errs = append(errs, *err)
		}
	}

	return errs
}

// PrintPreflightErrors formats and prints preflight errors to stderr.
// Returns true if there were errors.
func PrintPreflightErrors(errs []PreflightError) bool {
	if len(errs) == 0 {
		return false
	}

	fmt.Fprintln(os.Stderr)
	ui.PrintError(os.Stderr, "System checks failed:\n")
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  %s %s\n", ui.SymbolError, e.Message)
		fmt.Fprintf(os.Stderr, "    %s %s\n", ui.SymbolInfo, ui.MutedStyle.Render(e.Hint))
	}
	fmt.Fprintln(os.Stderr)

	return true
}

func checkLibvirtGroup() *PreflightError {
	u, err := user.Current()
	if err != nil {
		return nil // can't check, skip
	}

	gids, err := u.GroupIds()
	if err != nil {
		return nil
	}

	for _, gid := range gids {
		g, err := user.LookupGroupId(gid)
		if err != nil {
			continue
		}
		if g.Name == "libvirt" {
			return nil
		}
	}

	return &PreflightError{
		Message: fmt.Sprintf("user %q is not in the libvirt group", u.Username),
		Hint:    fmt.Sprintf("sudo usermod -aG libvirt %s && newgrp libvirt", u.Username),
	}
}

func checkStorageDir() *PreflightError {
	dir := "/var/lib/tank"

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return &PreflightError{
			Message: fmt.Sprintf("%s does not exist", dir),
			Hint:    "sudo mkdir -p /var/lib/tank && sudo chown root:libvirt /var/lib/tank && sudo chmod 2775 /var/lib/tank",
		}
	}
	if err != nil {
		return &PreflightError{
			Message: fmt.Sprintf("cannot access %s: %v", dir, err),
			Hint:    "sudo mkdir -p /var/lib/tank && sudo chown root:libvirt /var/lib/tank && sudo chmod 2775 /var/lib/tank",
		}
	}

	if !info.IsDir() {
		return &PreflightError{
			Message: fmt.Sprintf("%s exists but is not a directory", dir),
			Hint:    "Remove the file and recreate as a directory",
		}
	}

	// Check write access by trying to create a temp file
	testFile := dir + "/.tank-write-test"
	f, err := os.Create(testFile)
	if err != nil {
		return &PreflightError{
			Message: fmt.Sprintf("%s is not writable by current user", dir),
			Hint:    "sudo chown root:libvirt /var/lib/tank && sudo chmod 2775 /var/lib/tank",
		}
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

func checkLibvirtd() *PreflightError {
	// Check if libvirtd socket or service is active
	cmd := exec.Command("virsh", "-c", "qemu:///system", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := "cannot connect to libvirt (qemu:///system)"
		if strings.Contains(string(output), "Failed to connect") {
			msg = "libvirtd is not running"
		}
		return &PreflightError{
			Message: msg,
			Hint:    "sudo systemctl enable --now libvirtd",
		}
	}
	return nil
}
