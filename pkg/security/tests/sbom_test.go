// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests && trivy

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/avast/retry-go/v4"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSBOM(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID: "test_file_package",
			Expression: `open.file.path == "/usr/lib/os-release" && open.flags & O_CREAT != 0 && container.id != "" ` +
				`&& open.file.package.name == "base-files" && process.file.path != "" && process.file.package.name == "coreutils"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs, testOpts{enableSBOM: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu")
	if err != nil {
		t.Skip("Skipping sbom tests: Docker not available")
		return
	}
	defer dockerWrapper.stop()

	dockerWrapper.Run(t, "package-rule", func(t *testing.T, kind wrapperType, cmdFunc func(bin string, args, env []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			retry.Do(func() error {
				sbom := test.probe.GetResolvers().SBOMResolver.GetWorkload(dockerWrapper.containerID)
				if sbom == nil {
					return fmt.Errorf("failed to find SBOM for '%s'", dockerWrapper.containerID)
				}
				if !sbom.IsComputed() {
					return fmt.Errorf("report hasn't been generated for '%s'", dockerWrapper.containerID)
				}
				return nil
			})
			cmd := cmdFunc("/bin/touch", []string{"/usr/lib/os-release"}, nil)
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_file_package")
			assertFieldEqual(t, event, "open.file.package.name", "base-files")
			assertFieldEqual(t, event, "process.file.package.name", "coreutils")
			assertFieldNotEmpty(t, event, "container.id", "container id shouldn't be empty")

			test.validateOpenSchema(t, event)
		})
	})
}
