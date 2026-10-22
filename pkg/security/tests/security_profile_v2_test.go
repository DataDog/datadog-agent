// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var _ = declareInlineConfig(TestSecurityProfileV2Mounts)

func TestSecurityProfileV2Mounts(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableSecurityProfile: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	proc1Dir := filepath.Join(test.Root(), "proc1")
	if err := os.MkdirAll(proc1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("/proc/1", proc1Dir, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unix.Unmount(proc1Dir, unix.MNT_DETACH)
	}()

	dockerInstance, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	cmd := dockerInstance.Command("sleep", []string{"1"}, []string{})
	if _, err = cmd.CombinedOutput(); err != nil {
		t.Fatal(err)
	}

	p := test.probe.PlatformProbe.(*probe.EBPFProbe)
	tags, err := p.Resolvers.TagsResolver.ResolveWithErr(containerutils.ContainerID(dockerInstance.containerID))
	if err != nil {
		t.Fatal(err)
	}
	selector := &api.WorkloadSelectorMessage{Name: utils.GetTagValue("image_name", tags), Tag: "*"}

	// The workload never setns's, so every mount it reports lives in its base mount
	// namespace and must be flagged accordingly in the saved V2 profile.
	assert.Eventually(t, func() bool {
		msg, err := p.GetProfileManager().SaveSecurityProfile(&api.SecurityProfileSaveParams{Selector: selector})
		if err != nil || msg.GetError() != "" {
			return false
		}
		defer os.Remove(msg.GetFile())

		prof, err := DecodeSecurityProfile(msg.GetFile())
		if err != nil {
			return false
		}
		if len(prof.ActivityTree.Mounts) == 0 {
			return false
		}
		for _, mn := range prof.ActivityTree.Mounts {
			if !mn.InBaseNamespace {
				return false
			}
		}
		return true
	}, 30*time.Second, 500*time.Millisecond, "the workload mounts should be recorded in the base namespace")

	hostOnlyMount := t.TempDir()
	if err := unix.Mount("tmpfs", hostOnlyMount, "tmpfs", 0, ""); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unix.Unmount(hostOnlyMount, unix.MNT_DETACH)
	}()

	setnsCmd := exec.Command("docker", "exec", "--privileged", dockerInstance.containerName, "nsenter", "--mount="+filepath.Join(proc1Dir, "ns/mnt"), "sleep", "1")
	if out, err := setnsCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to nsenter host mount namespace: %v (%s)", err, string(out))
	}

	assert.Eventually(t, func() bool {
		msg, err := p.GetProfileManager().SaveSecurityProfile(&api.SecurityProfileSaveParams{Selector: selector})
		if err != nil || msg.GetError() != "" {
			return false
		}
		defer os.Remove(msg.GetFile())

		prof, err := DecodeSecurityProfile(msg.GetFile())
		if err != nil {
			return false
		}
		foundHostMount := false
		foundBaseMount := false
		for _, mn := range prof.ActivityTree.Mounts {
			if mn.MountPoint == hostOnlyMount {
				if mn.InBaseNamespace {
					return false
				}
				foundHostMount = true
			} else if mn.InBaseNamespace {
				foundBaseMount = true
			}
		}
		return foundHostMount && foundBaseMount
	}, 30*time.Second, 500*time.Millisecond, "mounts from a foreign namespace entered via setns should be seeded with base_namespace=false")
}
