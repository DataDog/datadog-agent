// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"go.yaml.in/yaml/v3"
)

// binaryIdentity records the installed files and immutable Docker image, not
// mutable worktree inputs or a requested tag. No-build apply requires this fact.
type binaryIdentity struct {
	ImageID string            `json:"imageID"`
	Files   map[string]string `json:"files"`
}

func pinnedFiles(entry envstore.Entry) (map[string]string, error) {
	result := map[string]string{}
	for _, name := range []string{"agent-binary", "dev-lib"} {
		err := filepath.Walk(filepath.Join(entry.Dir, name), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("pinned artifact must be a regular file")
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			hash := sha256.New()
			if _, err := io.Copy(hash, f); err != nil {
				return err
			}
			rel, err := filepath.Rel(entry.Dir, path)
			if err != nil {
				return err
			}
			result[rel] = hex.EncodeToString(hash.Sum(nil))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (b *Binary) dockerCommand(args ...string) (string, error) {
	if b.docker != nil {
		return b.docker(args...)
	}
	out, err := exec.Command("docker", args...).Output()
	// Docker errors can include config/env; never return stderr verbatim.
	if err != nil {
		return "", fmt.Errorf("docker %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (b *Binary) currentImage(entry envstore.Entry) (string, error) {
	image, err := b.dockerCommand("inspect", "--format", "{{.Image}}", localinfra.AgentContainer(entry.Name))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		return "", fmt.Errorf("installed container has no immutable image identity")
	}
	return image, nil
}

func (b *Binary) recordPins(entry envstore.Entry) error {
	image, err := b.currentImage(entry)
	if err != nil {
		return err
	}
	files, err := pinnedFiles(entry)
	if err != nil {
		return err
	}
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{"_agent_binary": binaryIdentity{ImageID: image, Files: files}})
}

// ApplyRouting is intentionally not Install or Update(skipBuild): it never
// builds, copies worktree files, pulls/loads an image or deploys workloads.
func (b *Binary) ApplyRouting(cfg *config.File, entry envstore.Entry) error {
	if cfg.Agent.Receiver == nil {
		return fmt.Errorf("receiver apply requires an explicit selection")
	}
	stored, err := entry.LoadConfig()
	if err != nil {
		return err
	}
	if stored.Agent.Install != cfg.Agent.Install {
		return fmt.Errorf("receiver apply cannot change installer")
	}
	var oldSection, newSection any
	if yaml.Unmarshal(stored.Agent.Section, &oldSection) != nil || yaml.Unmarshal(cfg.Agent.Section, &newSection) != nil {
		return fmt.Errorf("invalid installer section")
	}
	var oldBuild, newBuild any
	if stored.Agent.Build != nil {
		if err := yaml.Unmarshal(stored.Agent.Build.Section, &oldBuild); err != nil {
			return err
		}
		oldBuild = []any{stored.Agent.Build.Provider, oldBuild}
	}
	if cfg.Agent.Build != nil {
		if err := yaml.Unmarshal(cfg.Agent.Build.Section, &newBuild); err != nil {
			return err
		}
		newBuild = []any{cfg.Agent.Build.Provider, newBuild}
	}
	if !reflect.DeepEqual(oldBuild, newBuild) {
		return fmt.Errorf("receiver apply cannot change artifact source")
	}
	if !reflect.DeepEqual(oldSection, newSection) {
		return fmt.Errorf("receiver apply only changes agent.receiver; installer config/artifacts must match")
	}
	if !reflect.DeepEqual(stored.Workloads, cfg.Workloads) {
		return fmt.Errorf("receiver apply cannot change workloads")
	}
	section, err := decodeBinarySection(cfg)
	if err != nil {
		return err
	}
	agentYAML, err := b.prepareConfig(cfg, entry, section)
	if err != nil {
		return err
	}
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return err
	}
	if meta["_agent_artifact"] != nil {
		result, err := readArtifact(entry)
		if err != nil {
			return err
		}
		if err := result.RequireBinaryRouting(); err != nil {
			return err
		}
		if result.Binary == nil || result.Binary.RuntimeImageID == "" {
			return fmt.Errorf("verified binary runtime receipt required")
		}
		image, err := b.currentImage(entry)
		if err != nil {
			return err
		}
		if image != result.Binary.RuntimeImageID {
			return fmt.Errorf("running image differs from binary artifact receipt")
		}
		args := preparedBinaryRunArgs(entry, *result.Binary)
		if meta[runtimeStateKey] != nil {
			if err := b.verifyRuntimeState(entry); err != nil {
				return err
			}
		} else {
			if info, err := os.Stat(filepath.Join(entry.Dir, "agent-run")); err != nil || !info.IsDir() {
				return fmt.Errorf("persistent Agent runtime state missing")
			}
			args = preparedBinaryBindStateArgs(entry, *result.Binary)
		}
		if err := b.activatePreparedBinary(entry, section, agentYAML, args); err != nil {
			return err
		}
		if b.ready != nil {
			return b.ready(entry)
		}
		return b.waitForReady(entry)
	}
	return fmt.Errorf("legacy pin-only identity has no core-source routing capability evidence; explicitly rebuild with invoke-binary after any required runtime-state migration/recreation")
}
