package share

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the path to the tank share directory.
//
// Search order:
//  1. $TANK_SHARE_DIR (explicit override)
//  2. <binary-dir>/share/tank/ (sibling — portable tarball archives)
//  3. <binary-dir>/../share/tank/ (relative to binary — /usr/bin → /usr/share)
//  4. /usr/share/tank/ (system packages — deb/rpm)
func Dir() (string, error) {
	// 1. Environment override
	if env := os.Getenv("TANK_SHARE_DIR"); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env, nil
		}
		return "", fmt.Errorf("TANK_SHARE_DIR=%q does not exist or is not a directory", env)
	}

	// 2-3. Relative to binary
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
		if err == nil {
			binDir := filepath.Dir(exe)
			// Sibling: <dir>/tank + <dir>/share/tank/ (portable tarball)
			candidate := filepath.Join(binDir, "share", "tank")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, nil
			}
			// Parent: <dir>/bin/tank → <dir>/share/tank/ (system install)
			candidate = filepath.Join(binDir, "..", "share", "tank")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, nil
			}
		}
	}

	// 4. System path
	const systemPath = "/usr/share/tank"
	if info, err := os.Stat(systemPath); err == nil && info.IsDir() {
		return systemPath, nil
	}

	return "", fmt.Errorf("tank share directory not found (set TANK_SHARE_DIR or install tank system-wide)")
}

// LayerPath returns the absolute path to a named shared layer.
func LayerPath(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	p := filepath.Join(dir, "layers", name)
	if info, err := os.Stat(p); err != nil || !info.IsDir() {
		return "", fmt.Errorf("shared layer %q not found in %s", name, dir)
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}
