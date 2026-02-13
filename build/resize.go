package build

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type mountEntry struct {
	Mount string
	Device string
}

type fsEntry struct {
	Device string
	FSType string
}

type volumeEntry struct {
	VG string
	LV string
}

type lvmInfo struct {
	HasLVM bool
	PVs    []string
	LVs    []volumeEntry
}

type rootFS struct {
	Device string
	FSType string
}

func growRootFilesystem(imagePath, applianceDir string, progress io.Writer) error {
	mountpoints, filesystems, lvm, err := inspectGuest(imagePath, applianceDir)
	if err != nil {
		return err
	}

	root, err := findRootFilesystem(mountpoints, filesystems)
	if err != nil {
		return err
	}

	if root.FSType == "" {
		fmt.Fprintf(progress, "  %s Root filesystem type not detected; skipping resize\n", symbolDot)
		return nil
	}
	if root.FSType == "swap" || root.FSType == "unknown" {
		fmt.Fprintf(progress, "  %s Root filesystem type %s not supported; skipping resize\n", symbolDot, root.FSType)
		return nil
	}

	if lvm.HasLVM {
		return growRootFilesystemLVM(imagePath, applianceDir, root, lvm, progress)
	}

	partDevice, partNum, ok := parsePartitionDevice(root.Device)
	if !ok {
		return fmt.Errorf("root filesystem device %s is not a partition", root.Device)
	}

	return growRootFilesystemPartition(imagePath, applianceDir, root, partDevice, partNum, progress)
}

func inspectGuest(imagePath, applianceDir string) ([]mountEntry, []fsEntry, lvmInfo, error) {
	args := []string{"-a", imagePath, "-i"}
	cmd := exec.Command("guestfish", args...)
	env, err := guestfsEnv(applianceDir)
	if err != nil {
		return nil, nil, lvmInfo{}, err
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, lvmInfo{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, nil, lvmInfo{}, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, nil, lvmInfo{}, err
	}

	_, err = io.WriteString(stdin, strings.Join([]string{
		"echo MOUNTPOINTS",
		"mountpoints",
		"echo FILESYSTEMS",
		"list-filesystems",
		"echo PVS",
		"pvs",
		"echo LVS",
		"lvs",
		"exit",
	}, "\n")+"\n")
	stdin.Close()
	if err != nil {
		cmd.Wait()
		return nil, nil, lvmInfo{}, err
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		cmd.Wait()
		return nil, nil, lvmInfo{}, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, nil, lvmInfo{}, fmt.Errorf("guestfish inspect failed: %w: %s", err, output)
	}

	mountpoints, filesystems, lvm, err := parseGuestfishInspection(string(output))
	if err != nil {
		return nil, nil, lvmInfo{}, err
	}

	return mountpoints, filesystems, lvm, nil
}

func parseGuestfishInspection(output string) ([]mountEntry, []fsEntry, lvmInfo, error) {
	var mountpoints []mountEntry
	var filesystems []fsEntry
	var lvm lvmInfo
	section := ""

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch line {
		case "MOUNTPOINTS":
			section = "mountpoints"
			continue
		case "FILESYSTEMS":
			section = "filesystems"
			continue
		case "PVS":
			section = "pvs"
			lvm.HasLVM = true
			continue
		case "LVS":
			section = "lvs"
			lvm.HasLVM = true
			continue
		}

		switch section {
		case "mountpoints":
			entry, ok := parseMountEntry(line)
			if ok {
				mountpoints = append(mountpoints, entry)
			}
		case "filesystems":
			entry, ok := parseFilesystemEntry(line)
			if ok {
				filesystems = append(filesystems, entry)
			}
		case "pvs":
			if strings.HasPrefix(line, "/dev/") {
				lvm.PVs = append(lvm.PVs, line)
			}
		case "lvs":
			if strings.HasPrefix(line, "/dev/") {
				lvm.LVs = append(lvm.LVs, splitLV(line))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, lvmInfo{}, err
	}

	if len(mountpoints) == 0 {
		return nil, nil, lvmInfo{}, fmt.Errorf("failed to detect guest mountpoints")
	}
	if len(filesystems) == 0 {
		return nil, nil, lvmInfo{}, fmt.Errorf("failed to detect guest filesystems")
	}

	return mountpoints, filesystems, lvm, nil
}

func parseMountEntry(line string) (mountEntry, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return mountEntry{}, false
	}
	device := strings.TrimSpace(parts[0])
	mount := strings.TrimSpace(parts[1])
	if device == "" || mount == "" {
		return mountEntry{}, false
	}
	return mountEntry{Mount: mount, Device: device}, true
}

func parseFilesystemEntry(line string) (fsEntry, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return fsEntry{}, false
	}
	device := strings.TrimSpace(parts[0])
	fsType := strings.TrimSpace(parts[1])
	if device == "" || fsType == "" {
		return fsEntry{}, false
	}
	return fsEntry{Device: device, FSType: fsType}, true
}

func splitLV(device string) volumeEntry {
	trimmed := strings.TrimPrefix(device, "/dev/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return volumeEntry{}
	}
	return volumeEntry{VG: parts[0], LV: parts[1]}
}

func findRootFilesystem(mountpoints []mountEntry, filesystems []fsEntry) (rootFS, error) {
	rootDevice := ""
	for _, entry := range mountpoints {
		if entry.Mount == "/" {
			rootDevice = entry.Device
			break
		}
	}
	if rootDevice == "" {
		return rootFS{}, fmt.Errorf("root mountpoint not found in guest inspection")
	}

	for _, fs := range filesystems {
		if fs.Device == rootDevice {
			return rootFS{Device: fs.Device, FSType: fs.FSType}, nil
		}
	}

	return rootFS{Device: rootDevice}, nil
}

func parsePartitionDevice(device string) (string, int, bool) {
	for i := len(device) - 1; i >= 0; i-- {
		if device[i] < '0' || device[i] > '9' {
			if i == len(device)-1 {
				return "", 0, false
			}
			partNum, err := strconv.Atoi(device[i+1:])
			if err != nil {
				return "", 0, false
			}
			base := device[:i+1]
			if strings.HasSuffix(base, "p") && len(base) > 1 {
				base = strings.TrimSuffix(base, "p")
			}
			return base, partNum, true
		}
	}
	return "", 0, false
}

func growRootFilesystemPartition(imagePath, applianceDir string, root rootFS, partDevice string, partNum int, progress io.Writer) error {
	fmt.Fprintf(progress, "  %s Growing root partition %s\n", symbolDot, root.Device)
	cmd := exec.Command("guestfish", "-a", imagePath, "-i")
	env, err := guestfsEnv(applianceDir)
	if err != nil {
		return err
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return err
	}

	commands := []string{
		fmt.Sprintf("part-resize %s %d -1", partDevice, partNum),
	}
	commands = append(commands, rootResizeCommands(root.FSType, root.Device)...)
	commands = append(commands, "exit")
	_, err = io.WriteString(stdin, strings.Join(commands, "\n")+"\n")
	stdin.Close()
	if err != nil {
		cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("guestfish grow root partition failed: %w", err)
	}
	return nil
}

func growRootFilesystemLVM(imagePath, applianceDir string, root rootFS, lvm lvmInfo, progress io.Writer) error {
	if len(lvm.PVs) == 0 || len(lvm.LVs) == 0 {
		return fmt.Errorf("LVM detected but no PVs or LVs found")
	}

	primaryPV := lvm.PVs[0]
	partDevice, partNum, ok := parsePartitionDevice(primaryPV)
	if !ok {
		return fmt.Errorf("unsupported LVM PV device %s", primaryPV)
	}

	fmt.Fprintf(progress, "  %s Growing root LVM %s\n", symbolDot, root.Device)
	cmd := exec.Command("guestfish", "-a", imagePath, "-i")
	env, err := guestfsEnv(applianceDir)
	if err != nil {
		return err
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = progress
	cmd.Stderr = progress
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return err
	}

	commands := []string{
		fmt.Sprintf("part-resize %s %d -1", partDevice, partNum),
		fmt.Sprintf("pvresize %s", primaryPV),
		fmt.Sprintf("lvresize-free %s 100", root.Device),
	}
	commands = append(commands, rootResizeCommands(root.FSType, root.Device)...)
	commands = append(commands, "exit")

	_, err = io.WriteString(stdin, strings.Join(commands, "\n")+"\n")
	stdin.Close()
	if err != nil {
		cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("guestfish grow root LVM failed: %w", err)
	}
	return nil
}

func rootResizeCommands(fsType, device string) []string {
	switch fsType {
	case "ext2", "ext3", "ext4":
		return []string{fmt.Sprintf("resize2fs %s", device)}
	case "xfs":
		return []string{fmt.Sprintf("mount %s /", device), "xfs-growfs /", "umount /"}
	case "btrfs":
		return []string{fmt.Sprintf("mount %s /", device), "btrfs-filesystem-resize max /", "umount /"}
	default:
		return []string{}
	}
}
