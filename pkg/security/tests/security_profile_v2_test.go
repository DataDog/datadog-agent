// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
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
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()
	selector := &cgroupModel.WorkloadSelector{Image: "fake_v2_mounts", Tag: "latest"}
	fakeManualTagger.SpecifyNextSelector(selector)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                true,
		activityDumpRateLimiter:           200,
		activityDumpTracedCgroupsCount:    3,
		activityDumpDuration:              testActivityDumpDuration,
		activityDumpLocalStorageDirectory: outputDir,
		activityDumpLocalStorageFormats:   []string{"profile"},
		activityDumpTracedEventTypes:      []string{"exec", "open", "mount"},
		enableSecurityProfile:             true,
		securityProfileDir:                outputDir,
		securityProfileWatchDir:           true,
		tagger:                            fakeManualTagger,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("seeded-mounts", func(t *testing.T) {
		dockerInstance, err := test.StartADocker()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		// Let the workload link to its V2 profile and seed its mount table from the resolver.
		time.Sleep(6 * time.Second)

		p, ok := test.probe.PlatformProbe.(*probe.EBPFProbe)
		if !ok {
			t.Skip("not supported")
		}
		manager, ok := p.GetProfileManager().(*securityprofile.ManagerV2)
		if !ok {
			t.Fatal("V2 profile manager is not active")
		}

		prof := manager.GetProfile(cgroupModel.WorkloadSelector{Image: selector.Image, Tag: "*"})
		if prof == nil {
			t.Fatal("no V2 profile for the workload")
		}

		prof.Lock()
		defer prof.Unlock()

		if len(prof.ActivityTree.Mounts) == 0 {
			t.Fatal("expected the workload mount table to be seeded, got no mounts")
		}
	})
}
