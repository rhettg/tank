package build

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultBuildMemMB  = 8192
	defaultRootSize    = "50G"
	buildMemEnvVar     = "TANK_BUILD_MEM_MB"
	buildRootSizeEnv   = "TANK_BUILD_ROOT_SIZE"
	guestfsMemEnvVar   = "LIBGUESTFS_MEMSIZE"
)

func buildMemSizeMB() (int, error) {
	value := strings.TrimSpace(os.Getenv(buildMemEnvVar))
	if value == "" {
		return defaultBuildMemMB, nil
	}

	mem, err := strconv.Atoi(value)
	if err != nil || mem <= 0 {
		return 0, fmt.Errorf("invalid %s value %q", buildMemEnvVar, value)
	}
	return mem, nil
}

func resolveRootSize(rootSize string) (string, error) {
	if strings.TrimSpace(rootSize) != "" {
		return rootSize, nil
	}

	value := strings.TrimSpace(os.Getenv(buildRootSizeEnv))
	if value != "" {
		return value, nil
	}

	return defaultRootSize, nil
}

func guestfsEnv(applianceDir string) ([]string, error) {
	env := os.Environ()
	if applianceDir != "" {
		env = append(env, "LIBGUESTFS_PATH="+applianceDir)
	}
	if os.Getenv(guestfsMemEnvVar) == "" {
		mem, err := buildMemSizeMB()
		if err != nil {
			return nil, err
		}
		if mem > 0 {
			env = append(env, fmt.Sprintf("%s=%d", guestfsMemEnvVar, mem))
		}
	}
	return env, nil
}
