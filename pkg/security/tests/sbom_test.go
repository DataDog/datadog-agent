// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests && trivy

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var _ = declare(TestSBOM, testOpts{enableSBOM: true, enableHostSBOM: true})

func TestSBOM(t *testing.T) {
	t.Skip("this test is currently flaky, needs to be stabilized before re-enabling")

	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	kv, err := kernel.NewKernelVersion()
	if err != nil {
		t.Fatalf("failed to get kernel version: %s", err)
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
		{
			ID: "test_host_file_package",
			Expression: `open.file.path == "/usr/lib/os-release" && (open.flags & O_CREAT != 0) && (process.container.id == "") ` +
				`&& process.file.path != "" && process.file.package.name == "coreutils"`,
		},
	}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

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

	t.Run("host", func(t *testing.T) {
		flake.MarkOnJobName(t, "ubuntu_25.10")
		test.WaitSignalFromRule(t, func() error {
			sbom := p.Resolvers.SBOMResolver.GetWorkload("")
			if sbom == nil {
				return errors.New("failed to find host SBOM for host")
			}
			cmd := exec.Command("/bin/touch", "/usr/lib/os-release")
			return cmd.Run()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_host_file_package")
			assertFieldEqual(t, event, "process.file.package.name", "coreutils")
			assertFieldEqual(t, event, "process.container.id", "", "container id should be empty")

			if kv.IsUbuntuKernel() || kv.IsDebianKernel() {
				checkVersionAgainstApt(t, event, "coreutils")
			}
			if kv.IsRH7Kernel() || kv.IsRH8Kernel() || kv.IsRH9Kernel() || kv.IsAmazonLinuxKernel() || kv.IsSuseKernel() {
				checkVersionAgainstRpm(t, event, "coreutils")
			}

			test.validateOpenSchema(t, event)
		}, "test_host_file_package")
	})
}

func checkVersionAgainstApt(tb testing.TB, event *model.Event, pkgName string) {
	version, _ := event.GetFieldValue("process.file.package.version")
	release, _ := event.GetFieldValue("process.file.package.release")
	epoch, _ := event.GetFieldValue("process.file.package.epoch")
	v := buildDebianVersion(version.(string), release.(string), epoch.(int))

	out, err := exec.Command("apt-cache", "policy", pkgName).CombinedOutput()
	require.NoError(tb, err, "failed to get package version: %s", string(out))

	assert.Contains(tb, string(out), "Installed: "+v, "package version doesn't match")
}

func buildDebianVersion(version, release string, epoch int) string {
	v := version + "-" + release
	if epoch > 0 {
		v = strconv.Itoa(epoch) + ":" + v
	}
	return v
}

func checkVersionAgainstRpm(tb testing.TB, event *model.Event, pkgName string) {
	version, _ := event.GetFieldValue("process.file.package.version")
	release, _ := event.GetFieldValue("process.file.package.release")
	epoch, _ := event.GetFieldValue("process.file.package.epoch")

	out, err := exec.Command("rpm", "-q", "--queryformat", "%{VERSION}", pkgName).CombinedOutput()
	require.NoError(tb, err, "failed to get package version: %s", string(out))
	assert.Equal(tb, string(out), version, "package version doesn't match")

	out, err = exec.Command("rpm", "-q", "--queryformat", "%{RELEASE}", pkgName).CombinedOutput()
	require.NoError(tb, err, "failed to get package version: %s", string(out))
	assert.Equal(tb, string(out), release, "package release doesn't match")

	out, err = exec.Command("rpm", "-q", "--queryformat", "%{EPOCH}", pkgName).CombinedOutput()
	require.NoError(tb, err, "failed to get package version: %s", string(out))
	if string(out) != "(none)" {
		assert.Equal(tb, string(out), fmt.Sprintf("%d", epoch), "package epoch doesn't match")
	}
}

var _ = declare(TestSBOMScriptInterpreterInUse, testOpts{enableSBOM: true})

// TestSBOMScriptInterpreterInUse checks that executing a shebang script attributes
// the package shipping the interpreter. The exec event carries the script as
// exec.file, and no package owns it, so the interpreter is only reachable through
// the linux_binprm field. No rule here mentions a package field, which keeps the
// SBOM hook on the exec event the only thing that can stamp the interpreter.
func TestSBOMScriptInterpreterInUse(t *testing.T) {
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
			ID:         "test_sbom_interpreter_placeholder",
			Expression: `open.file.path == "/tmp/test-sbom-interpreter-never-opened"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	// the test root is bind-mounted into the container at the same path, so the
	// script lives outside the image and belongs to no package
	scriptPath, _, err := test.Path("sbom-interpreter.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(scriptPath)

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "ubuntu", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}

	// dash owns /bin/sh and the /usr/bin/dash it resolves to on Debian and Ubuntu
	const interpreterPkg = "dash"

	dockerWrapper.Run(t, "interpreter-package", func(t *testing.T, _ wrapperType, cmdFunc func(bin string, args, env []string) *exec.Cmd) {
		containerID := containerutils.ContainerID(dockerWrapper.containerID)

		if err := retry(t, func() error {
			workload := p.Resolvers.SBOMResolver.GetWorkload(containerID)
			if workload == nil {
				return fmt.Errorf("failed to find SBOM for '%s'", containerID)
			}
			if !workload.IsComputed() {
				return fmt.Errorf("report hasn't been generated for '%s'", containerID)
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(50)); err != nil {
			t.Fatal(err)
		}

		// the container runs sleep, so nothing has gone through the interpreter yet
		before, held := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, interpreterPkg)
		if !held {
			t.Skipf("image %s does not ship the %s package", dockerWrapper.image, interpreterPkg)
		}
		require.True(t, before.IsZero(), "%s was already attributed before the script ran", interpreterPkg)

		if out, err := cmdFunc(scriptPath, nil, nil).CombinedOutput(); err != nil {
			t.Fatalf("failed to run %s in the container: %v, %s", scriptPath, err, out)
		}

		if err := retry(t, func() error {
			last, held := p.Resolvers.SBOMResolver.LastPackageAccess(containerID, interpreterPkg)
			if !held {
				return fmt.Errorf("workload '%s' no longer holds the %s package", containerID, interpreterPkg)
			}
			if last.IsZero() {
				return fmt.Errorf("running a shebang script left the %s package unattributed", interpreterPkg)
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(200*time.Millisecond)), backoff.WithMaxTries(25)); err != nil {
			t.Error(err)
		}
	})
}
