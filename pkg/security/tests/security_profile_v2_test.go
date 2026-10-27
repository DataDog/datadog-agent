// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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
}
