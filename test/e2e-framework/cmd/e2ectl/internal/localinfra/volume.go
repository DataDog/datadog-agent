// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const runtimeOwnerLabel = "com.datadoghq.e2ectl.runtime-owner"

// RuntimeVolume is mutable environment state, separate from artifact receipts.
// Its deterministic identity also makes failed-install cleanup recoverable.
type RuntimeVolume struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

func AgentRuntimeVolume(envDir string, createdAt time.Time) RuntimeVolume {
	sum := sha256.Sum256([]byte(filepath.Clean(envDir) + "\x00" + createdAt.UTC().Format(time.RFC3339Nano)))
	owner := hex.EncodeToString(sum[:])
	return RuntimeVolume{Name: "e2ectl-agent-runtime-" + owner[:32], Owner: owner}
}

type DockerCommand func(...string) (string, error)

func volumeCommand(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return "", fmt.Errorf("docker volume operation failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
func volumeExists(v RuntimeVolume, run DockerCommand) (bool, error) {
	out, err := run("volume", "ls", "--filter", "name="+v.Name, "--format", "{{.Name}}")
	if err != nil {
		return false, err
	}
	for _, name := range strings.Fields(out) {
		if name == v.Name {
			return true, nil
		}
	}
	return false, nil
}
func verifyVolumeOwner(v RuntimeVolume, run DockerCommand) error {
	out, err := run("volume", "inspect", "--format", "{{json .Labels}}", v.Name)
	if err != nil {
		return err
	}
	var labels map[string]string
	if err = json.Unmarshal([]byte(out), &labels); err != nil {
		return err
	}
	if labels[runtimeOwnerLabel] != v.Owner {
		return fmt.Errorf("runtime volume %s belongs to another owner; refusing reuse/removal", v.Name)
	}
	return nil
}
func validateRuntimeVolume(v RuntimeVolume) error {
	if len(v.Owner) != 64 {
		return fmt.Errorf("invalid runtime volume ownership")
	}
	if _, err := hex.DecodeString(v.Owner); err != nil {
		return err
	}
	if v.Name != "e2ectl-agent-runtime-"+v.Owner[:32] {
		return fmt.Errorf("invalid runtime volume name")
	}
	return nil
}
func EnsureRuntimeVolume(v RuntimeVolume, run DockerCommand) error {
	if err := validateRuntimeVolume(v); err != nil {
		return err
	}
	if run == nil {
		run = volumeCommand
	}
	exists, err := volumeExists(v, run)
	if err != nil {
		return err
	}
	if !exists {
		if _, err = run("volume", "create", "--label", runtimeOwnerLabel+"="+v.Owner, v.Name); err != nil {
			return err
		}
	}
	return verifyVolumeOwner(v, run)
}
func VerifyRuntimeVolume(v RuntimeVolume, run DockerCommand) error {
	if err := validateRuntimeVolume(v); err != nil {
		return err
	}
	if run == nil {
		run = volumeCommand
	}
	exists, err := volumeExists(v, run)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("recorded Agent runtime volume %s is missing; refusing empty-state fallback", v.Name)
	}
	return verifyVolumeOwner(v, run)
}

// RemoveAgentAndRuntime stops the process before deleting its owned state.
func RemoveAgentAndRuntime(container string, v RuntimeVolume, run DockerCommand) error {
	if err := validateRuntimeVolume(v); err != nil {
		return err
	}
	if run == nil {
		run = volumeCommand
	}
	if _, err := run("rm", "-f", container); err != nil {
		return fmt.Errorf("removing Agent before runtime volume: %w", err)
	}
	return RemoveRuntimeVolume(v, run)
}

// RemoveRuntimeVolume never prunes or removes an unowned/in-use volume. The
// caller must remove its Agent container first, and retain its entry on error.
func RemoveRuntimeVolume(v RuntimeVolume, run DockerCommand) error {
	if err := validateRuntimeVolume(v); err != nil {
		return err
	}
	if run == nil {
		run = volumeCommand
	}
	exists, err := volumeExists(v, run)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err = verifyVolumeOwner(v, run); err != nil {
		return err
	}
	_, err = run("volume", "rm", v.Name)
	return err
}
