// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests && trivy

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"

	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// sbomTestOpts is the config of the SBOM tests, which share one module.
var sbomTestOpts = testOpts{enableSBOM: true, enableHostSBOM: true}

var _ = declare(TestSBOM, sbomTestOpts)

func TestSBOM(t *testing.T) {
	t.Skip("this test is currently flaky, needs to be stabilized before re-enabling")

	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID: "test_file_package",
			Expression: `open.file.path == "/usr/lib/os-release" && (open.flags & O_CREAT != 0) && (process.container.id != "") ` +
				`&& open.file.package.name == "base-files" && process.file.path != "" && process.file.package.name == "coreutils"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}

	var sbomResult *sbompkg.ScanResult
	dockerWrapper.Run(t, "package-rule", func(t *testing.T, _ wrapperType, cmdFunc func(bin string, args, env []string) *exec.Cmd) {
		if err := p.Resolvers.SBOMResolver.RegisterListener(sbom.SBOMComputed, func(sbom *sbompkg.ScanResult) {
			sbomResult = sbom
		}); err != nil {
			t.Fatal(err)
		}

		test.WaitSignalFromRule(t, func() error {
			retry(t, func() error {
				sbom := p.Resolvers.SBOMResolver.GetWorkload(containerutils.ContainerID(dockerWrapper.containerID))
				if sbom == nil {
					return fmt.Errorf("failed to find SBOM for '%s'", dockerWrapper.containerID)
				}
				if !sbom.IsComputed() {
					return fmt.Errorf("report hasn't been generated for '%s'", dockerWrapper.containerID)
				}
				return nil
			}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(10))
			cmd := cmdFunc("/bin/touch", []string{"/usr/lib/os-release"}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_file_package")
			assertFieldEqual(t, event, "open.file.package.name", "base-files")
			assertFieldEqual(t, event, "process.file.package.name", "coreutils")
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")
			assertFieldNotEmpty(t, event, "container.id", "container id shouldn't be empty")
			assert.NotNil(t, sbomResult, "sbom result should not be nil")
			assert.Equal(t, sbomResult.Error, nil, "sbom result should not have an error")
			assert.Equal(t, sbomResult.RequestID, dockerWrapper.containerID, "sbom result should have the same request id as the container id")
			cyclonedx := sbomResult.Report.ToCycloneDX()
			assert.NotNil(t, cyclonedx, "sbom result should not be nil")
			assert.NotZero(t, len(cyclonedx.Components))
			test.validateOpenSchema(t, event)
		}, "test_file_package")
	})
}
