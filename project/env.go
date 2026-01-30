package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnvFile reads a .env file from the project root and returns
// the variables as a slice of "KEY=VALUE" strings.
// Returns an empty slice if the file doesn't exist.
func LoadEnvFile(projectRoot string) ([]string, error) {
	envPath := filepath.Join(projectRoot, ".env")

	f, err := os.Open(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var envVars []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Must contain =
		if !strings.Contains(line, "=") {
			continue
		}

		// Parse KEY=VALUE (handle quoted values)
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		envVars = append(envVars, key+"="+value)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return envVars, nil
}
