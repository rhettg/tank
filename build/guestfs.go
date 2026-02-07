package build

import (
        "errors"
        "fmt"
        "io"
        "net/http"
        "os"
        "os/exec"
        "path/filepath"
        "strings"
        "syscall"
)

const guestfsApplianceBaseURL = "https://download.libguestfs.org/binaries/appliance"

// EnsureGuestfsAppliance ensures a usable libguestfs fixed appliance exists.
// Returns the appliance directory to use via LIBGUESTFS_PATH, or an empty
// string to fall back to supermin.
func EnsureGuestfsAppliance(progress io.Writer) (string, error) {
        version, err := guestfsVersion()
        if err != nil {
                return "", err
        }

        cacheDir, err := guestfsCacheDir(version)
        if err != nil {
                return "", err
        }

        if hasAppliance(cacheDir) {
                fmt.Fprintf(progress, "  %s Guestfs appliance cached %s\n", symbolDot, mutedStyle.Render(cacheDir))
                return cacheDir, nil
        }

        if systemDir := systemApplianceDir(); systemDir != "" {
                fmt.Fprintf(progress, "  %s Guestfs appliance system %s\n", symbolDot, mutedStyle.Render(systemDir))
                return systemDir, nil
        }

        if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
                return "", fmt.Errorf("creating guestfs cache directory: %w", err)
        }

        if !kernelReadable() {
                fmt.Fprintf(progress, "  %s Host kernel unreadable; skipping local build and downloading prebuilt appliance\n", symbolDot)
                if err := downloadAppliance(version, cacheDir, progress); err != nil {
                        return "", err
                }
                fmt.Fprintf(progress, "  %s Guestfs appliance downloaded %s\n", symbolSuccess, mutedStyle.Render(cacheDir))
                return cacheDir, nil
        }

        fmt.Fprintf(progress, "  %s Guestfs appliance missing; attempting local build\n", symbolDot)

        if _, err := exec.LookPath("libguestfs-make-fixed-appliance"); err == nil {
                fmt.Fprintf(progress, "  %s Building guestfs appliance\n", symbolDot)
                if err := buildFixedAppliance(cacheDir, progress); err == nil {
                        fmt.Fprintf(progress, "  %s Guestfs appliance built %s\n", symbolSuccess, mutedStyle.Render(cacheDir))
                        return cacheDir, nil
                }
                fmt.Fprintf(progress, "  %s Failed to build guestfs appliance, falling back to prebuilt download\n", symbolDot)
        } else {
                fmt.Fprintf(progress, "  %s libguestfs-make-fixed-appliance not available, falling back to prebuilt download\n", symbolDot)
        }

        fmt.Fprintf(progress, "  %s Downloading guestfs appliance\n", symbolDot)
        if err := downloadAppliance(version, cacheDir, progress); err != nil {
                fmt.Fprintf(progress, "  %s Download failed (%v); falling back to supermin appliance\n", symbolDot, err)
                return "", nil
        }
        fmt.Fprintf(progress, "  %s Guestfs appliance downloaded %s\n", symbolSuccess, mutedStyle.Render(cacheDir))
        return cacheDir, nil
}

func guestfsVersion() (string, error) {
        cmd := exec.Command("virt-customize", "--version")
        output, err := cmd.Output()
        if err != nil {
                return "", fmt.Errorf("reading virt-customize version: %w", err)
        }
        fields := strings.Fields(string(output))
        if len(fields) < 2 {
                return "", fmt.Errorf("unexpected virt-customize version output: %q", strings.TrimSpace(string(output)))
        }
        return fields[len(fields)-1], nil
}

func guestfsCacheDir(version string) (string, error) {
        cacheDir, err := CacheDir()
        if err != nil {
                return "", err
        }
        return filepath.Join(cacheDir, "guestfs", "appliance-"+version), nil
}

func systemApplianceDir() string {
        candidates := []string{
                "/usr/lib/libguestfs/appliance",
                "/usr/lib/guestfs/appliance",
                "/usr/local/lib/guestfs/appliance",
        }
        for _, dir := range candidates {
                if hasAppliance(dir) {
                        return dir
                }
        }
        if p := os.Getenv("LIBGUESTFS_PATH"); p != "" && hasAppliance(p) {
                return p
        }
        return ""
}

func hasAppliance(dir string) bool {
        if dir == "" {
                return false
        }
        for _, name := range []string{"kernel", "initrd", "root"} {
                if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
                        return false
                }
        }
        return true
}

func buildFixedAppliance(dir string, progress io.Writer) error {
        if err := os.RemoveAll(dir); err != nil {
                return err
        }
        cmd := exec.Command("libguestfs-make-fixed-appliance", dir)
        cmd.Stdout = progress
        cmd.Stderr = progress
        if err := cmd.Run(); err != nil {
                return fmt.Errorf("libguestfs-make-fixed-appliance: %w", err)
        }
        if !hasAppliance(dir) {
                return errors.New("fixed appliance missing expected files")
        }
        return nil
}

func downloadAppliance(version string, destDir string, progress io.Writer) error {
        versions := applianceVersions(version)
        if len(versions) == 0 {
                return fmt.Errorf("cannot derive appliance version from %q", version)
        }

        var lastErr error
        for _, candidate := range versions {
                err := downloadApplianceVersion(candidate, destDir, progress)
                if err == nil {
                        return nil
                }
                if errors.Is(err, errApplianceNotFound) {
                        lastErr = err
                        continue
                }
                return err
        }
        if lastErr != nil {
                return lastErr
        }
        return fmt.Errorf("unable to download guestfs appliance")
}

var errApplianceNotFound = errors.New("appliance not found")

func downloadApplianceVersion(version string, destDir string, progress io.Writer) error {
        if err := os.RemoveAll(destDir); err != nil {
                return err
        }
        if err := os.MkdirAll(destDir, 0755); err != nil {
                return err
        }

        tarball := fmt.Sprintf("appliance-%s.tar.xz", version)
        url := fmt.Sprintf("%s/%s", guestfsApplianceBaseURL, tarball)

        resp, err := http.Get(url)
        if err != nil {
                return fmt.Errorf("downloading guestfs appliance: %w", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode == http.StatusNotFound {
                return fmt.Errorf("%w: %s", errApplianceNotFound, url)
        }
        if resp.StatusCode != http.StatusOK {
                return fmt.Errorf("download failed: %s", resp.Status)
        }

        tmpTar := destDir + ".tar.xz"
        out, err := os.Create(tmpTar)
        if err != nil {
                return err
        }
        _, err = copyWithProgress(out, resp.Body, resp.ContentLength, progress)
        out.Close()
        if err != nil {
                os.Remove(tmpTar)
                return err
        }

        tmpDir := destDir + ".tmp"
        if err := os.RemoveAll(tmpDir); err != nil {
                os.Remove(tmpTar)
                return err
        }
        if err := os.MkdirAll(tmpDir, 0755); err != nil {
                os.Remove(tmpTar)
                return err
        }

        cmd := exec.Command("tar", "-xJf", tmpTar, "-C", tmpDir)
        cmd.Stdout = progress
        cmd.Stderr = progress
        if err := cmd.Run(); err != nil {
                os.Remove(tmpTar)
                os.RemoveAll(tmpDir)
                return fmt.Errorf("extracting guestfs appliance: %w", err)
        }

        extracted := filepath.Join(tmpDir, "appliance")
        if !hasAppliance(extracted) {
                os.Remove(tmpTar)
                os.RemoveAll(tmpDir)
                return errors.New("downloaded appliance missing expected files")
        }

        if err := os.Rename(extracted, destDir); err != nil {
                os.Remove(tmpTar)
                os.RemoveAll(tmpDir)
                return err
        }

        os.Remove(tmpTar)
        os.RemoveAll(tmpDir)
        return nil
}

func applianceVersions(version string) []string {
        parts := strings.Split(version, ".")
        if len(parts) < 2 {
                return nil
        }
        major := parts[0]
        minor := parts[1]
        var versions []string
        versions = append(versions, fmt.Sprintf("%s.%s.%s", major, minor, "0"))
        if len(parts) >= 3 {
                versions = append(versions, fmt.Sprintf("%s.%s.%s", major, minor, parts[2]))
        }
        versions = append(versions, fmt.Sprintf("%s.%s", major, minor))
        versions = uniqueStrings(versions)
        return versions
}

func uniqueStrings(values []string) []string {
        seen := make(map[string]struct{}, len(values))
        var out []string
        for _, value := range values {
                if _, ok := seen[value]; ok {
                        continue
                }
                seen[value] = struct{}{}
                out = append(out, value)
        }
        return out
}

func kernelReadable() bool {
        matches, _ := filepath.Glob("/boot/vmlinuz-*")
        if len(matches) == 0 {
                matches, _ = filepath.Glob("/boot/vmlinuz*")
        }
        if len(matches) == 0 {
                return true
        }
        for _, kernel := range matches {
                if err := syscall.Access(kernel, syscall.O_RDONLY); err == nil {
                        return true
                }
        }
        return false
}
