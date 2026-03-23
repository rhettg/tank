package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/rhettg/tank/project"
)

const metadataVersion = 1

type artifactMetadata struct {
	Version   int                       `json:"version"`
	Artifacts map[string]artifactRecord `json:"artifacts"`
	Builds    []buildRecord             `json:"builds"`
	Pins      []string                  `json:"pins,omitempty"`
}

type artifactRecord struct {
	Hash       string `json:"hash"`
	ParentHash string `json:"parent_hash,omitempty"`
}

type buildRecord struct {
	ProjectRoot string    `json:"project_root"`
	ProjectName string    `json:"project_name"`
	FinalHash   string    `json:"final_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

func metadataPath() (string, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "metadata", "artifacts.json"), nil
}

func metadataLockPath() (string, error) {
	cacheDir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "metadata", "artifacts.lock"), nil
}

func withMetadataLock(fn func() error) error {
	lockPath, err := metadataLockPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return err
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking metadata: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return fn()
}

func loadMetadata() (*artifactMetadata, error) {
	path, err := metadataPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &artifactMetadata{
			Version:   metadataVersion,
			Artifacts: make(map[string]artifactRecord),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var meta artifactMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata: %w", err)
	}
	if meta.Artifacts == nil {
		meta.Artifacts = make(map[string]artifactRecord)
	}
	if meta.Pins == nil {
		meta.Pins = []string{}
	}
	if meta.Version == 0 {
		meta.Version = metadataVersion
	}
	return &meta, nil
}

func saveMetadata(meta *artifactMetadata) error {
	path, err := metadataPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func recordBuildArtifacts(p *project.Project, stages []project.BuildStage, finalHash string) error {
	return withMetadataLock(func() error {
		meta, err := loadMetadata()
		if err != nil {
			return err
		}

		for i, stage := range stages {
			record := artifactRecord{Hash: stage.Hash}
			if i > 0 {
				record.ParentHash = stages[i-1].Hash
			}
			meta.Artifacts[stage.Hash] = record
		}

		createdAt := time.Now().UTC()
		found := false
		for i := range meta.Builds {
			record := &meta.Builds[i]
			if record.ProjectRoot == p.Root && record.FinalHash == finalHash {
				record.ProjectName = filepath.Base(p.Root)
				record.CreatedAt = createdAt
				found = true
				break
			}
		}
		if !found {
			meta.Builds = append(meta.Builds, buildRecord{
				ProjectRoot: p.Root,
				ProjectName: filepath.Base(p.Root),
				FinalHash:   finalHash,
				CreatedAt:   createdAt,
			})
		}

		sort.Slice(meta.Builds, func(i, j int) bool {
			if meta.Builds[i].ProjectRoot == meta.Builds[j].ProjectRoot {
				return meta.Builds[i].CreatedAt.Before(meta.Builds[j].CreatedAt)
			}
			return meta.Builds[i].ProjectRoot < meta.Builds[j].ProjectRoot
		})

		return saveMetadata(meta)
	})
}

func PinBuild(hash string) error {
	return withMetadataLock(func() error {
		meta, err := loadMetadata()
		if err != nil {
			return err
		}
		for _, pinned := range meta.Pins {
			if pinned == hash {
				return nil
			}
		}
		meta.Pins = append(meta.Pins, hash)
		sort.Strings(meta.Pins)
		return saveMetadata(meta)
	})
}

func UnpinBuild(hash string) error {
	return withMetadataLock(func() error {
		meta, err := loadMetadata()
		if err != nil {
			return err
		}
		var pins []string
		for _, pinned := range meta.Pins {
			if pinned != hash {
				pins = append(pins, pinned)
			}
		}
		meta.Pins = pins
		return saveMetadata(meta)
	})
}

func IsPinned(hash string) (bool, error) {
	var pinned bool
	err := withMetadataLock(func() error {
		meta, err := loadMetadata()
		if err != nil {
			return err
		}
		for _, candidate := range meta.Pins {
			if candidate == hash {
				pinned = true
				break
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return pinned, nil
}
