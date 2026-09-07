// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests && trivy

// Package tests holds tests related files
package tests

import (
	"errors"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/require"
	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var _ = declare(TestSBOMRuntimeEvidence, testOpts{enableSBOM: true})

type functionalSBOMSource struct {
	indexes chan *usage.Index
}

func newFunctionalSBOMSource() *functionalSBOMSource {
	return &functionalSBOMSource{indexes: make(chan *usage.Index, 1)}
}

func (s *functionalSBOMSource) Indexes() <-chan *usage.Index { return s.indexes }
func (s *functionalSBOMSource) Capabilities() (usage.Capabilities, bool) {
	return usage.Capabilities{Container: true}, true
}
func (s *functionalSBOMSource) Report(*usage.Report) error { return nil }
func (s *functionalSBOMSource) Refresh(usage.ScanID, containerutils.ContainerID) error {
	return nil
}

// TestSBOMRuntimeEvidence exercises the real eBPF event path. A package owning
// an executable mapping becomes used without being execve's target, while an
// ordinary read of a package-owned data file remains lookup-only.
func TestSBOMRuntimeEvidence(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("test needs to start a nested container")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("docker is unavailable")
	}

	source := newFunctionalSBOMSource()
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_sbom_data_open",
		Expression: `open.file.path == "/etc/passwd" && process.container.id != ""`,
	}}
	test, err := newTestModule(t, nil, ruleDefs, withSBOMIndexSource(source))
	require.NoError(t, err)
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("eBPF probe is unsupported")
	}

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	require.NoError(t, err)
	dockerWrapper.Run(t, "mmap-and-data-open", func(t *testing.T, _ wrapperType, cmdFunc func(string, []string, []string) *exec.Cmd) {
		libPaths := packagePaths(t, cmdFunc, "libtinfo6", "libtinfo.so.6")
		containerID := containerutils.ContainerID(dockerWrapper.containerID)
		source.indexes <- runtimeEvidenceIndex(containerID, libPaths)

		require.NoError(t, retry(t, func() error {
			_, dataHeld := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, "base-passwd")
			_, mmapHeld := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, "libtinfo6")
			if !dataHeld || !mmapHeld {
				return errors.New("runtime evidence index is not active yet")
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(10*time.Second)))

		test.WaitSignalFromRule(t, func() error {
			return cmdFunc("/bin/cat", []string{"/etc/passwd"}, nil).Run()
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_sbom_data_open")
		}, "test_sbom_data_open")
		// Rule callbacks run before internal SBOM observation in DispatchEvent.
		// Give that same event time to finish before asserting the negative.
		time.Sleep(250 * time.Millisecond)
		last, held := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, "base-passwd")
		require.True(t, held)
		require.Truef(t, last.IsZero(), "ordinary data read marked base-passwd used at %s", last)

		out, err := cmdFunc("/bin/bash", []string{"-c", "true"}, nil).CombinedOutput()
		require.NoErrorf(t, err, "failed to execute bash: %s", out)
		require.NoError(t, retry(t, func() error {
			last, held := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, "libtinfo6")
			if !held {
				return errors.New("container index no longer contains libtinfo6")
			}
			if last.IsZero() {
				return errors.New("bash executable mapping left libtinfo6 idle")
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(100*time.Millisecond)), backoff.WithMaxElapsedTime(10*time.Second)))
	})
}

func packagePaths(t *testing.T, cmdFunc func(string, []string, []string) *exec.Cmd, pkg, contains string) []string {
	t.Helper()
	out, err := cmdFunc("/usr/bin/dpkg-query", []string{"-L", pkg}, nil).CombinedOutput()
	require.NoErrorf(t, err, "failed to list %s files: %s", pkg, out)

	var paths []string
	for _, path := range strings.Fields(string(out)) {
		if strings.Contains(path, contains) {
			paths = append(paths, path)
		}
	}
	require.NotEmptyf(t, paths, "%s lists no path containing %q", pkg, contains)
	return paths
}

func runtimeEvidenceIndex(containerID containerutils.ContainerID, libPaths []string) *usage.Index {
	type entry struct {
		hash uint64
		ref  uint32
	}
	entries := []entry{{hash: murmur3.StringSum64("/etc/passwd"), ref: 0}}
	for _, path := range libPaths {
		entries = append(entries, entry{hash: murmur3.StringSum64(path), ref: 1})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].hash < entries[j].hash })

	index := &usage.Index{
		Scan:       usage.ContainerScan(string(containerID)),
		Generation: 1,
		IndexID:    "urn:uuid:functional-runtime-evidence",
		Status:     usage.Ready,
		Components: []usage.Component{
			{Name: "base-passwd", Reportable: true},
			{Name: "libtinfo6", Reportable: true},
		},
	}
	for _, entry := range entries {
		index.Hashes = append(index.Hashes, entry.hash)
		index.Refs = append(index.Refs, entry.ref)
		index.Activations = append(index.Activations, false)
	}
	return index
}

var _ sbom.IndexSource = (*functionalSBOMSource)(nil)
