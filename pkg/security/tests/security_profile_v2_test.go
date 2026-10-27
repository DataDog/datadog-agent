// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
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

var _ = declareInlineConfig(TestSecurityProfileV2SyscallSampling)

// TestSecurityProfileV2SyscallSampling checks that the syscalls sampled for workload profiles v2
// land on the process node that made them, whichever way the kernel reports them: while the
// process runs, when it exits, or when it execs.
func TestSecurityProfileV2SyscallSampling(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)
	checkKernelCompatibility(t, "Missing bpf_for_each_map_elem helper", func(kv *kernel.Version) bool {
		return !kv.HasBPFForEachMapElemHelper()
	})

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.eventSamplingSyscallsEnabled = true
	opts.workloadProfileEventTypes = []string{"exec", "syscalls"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// each subtest gets its own image, and so its own profile
	t.Run("running-process", func(t *testing.T) {
		dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "latest")

		// the process still sleeps when its syscalls are checked, so they can only come from the
		// periodic report
		cmd := dockerInstance.Command(workloadProfileSyscallTester, []string{"prctl-setname", "sampled", ";", "sleep", "30"}, nil)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		expectProfileSyscalls(t, test, selector, func(syscalls profileSyscalls) error {
			return syscalls.expect(workloadProfileSyscallTester, model.SysPrctl)
		})
	})

	t.Run("exit", func(t *testing.T) {
		dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "latest")

		// the process exits right after the syscall, before the first periodic report
		if out, err := dockerInstance.Command(workloadProfileSyscallTester, []string{"bind", "AF_INET", "any", "tcp"}, nil).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}

		expectProfileSyscalls(t, test, selector, func(syscalls profileSyscalls) error {
			return syscalls.expect(workloadProfileSyscallTester, model.SysBind, model.SysExitGroup)
		})
	})

	t.Run("exec", func(t *testing.T) {
		dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "latest")

		// the process execs right after the syscall, before the first periodic report: the syscall
		// belongs to the node of the process before the exec, not to the executed binary
		if out, err := dockerInstance.Command(workloadProfileSyscallTester, []string{"prctl-setname", "sampled", ";", "exec", "/bin/true"}, nil).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}

		expectProfileSyscalls(t, test, selector, func(syscalls profileSyscalls) error {
			if err := syscalls.expect(workloadProfileSyscallTester, model.SysPrctl, model.SysExecve); err != nil {
				return err
			}
			if err := syscalls.expect("/bin/true", model.SysExitGroup); err != nil {
				return err
			}
			if syscalls["/bin/true"][int(model.SysPrctl)] {
				return errors.New("prctl was reported on the node of the executed binary")
			}
			return nil
		})
	})
}

// profileSyscalls holds the syscalls of the process nodes of a profile, by argv0
type profileSyscalls map[string]map[int]bool

func (ps profileSyscalls) expect(argv0 string, syscalls ...model.Syscall) error {
	got, ok := ps[argv0]
	if !ok {
		return fmt.Errorf("no process node for %s", argv0)
	}
	for _, s := range syscalls {
		if !got[int(s)] {
			return fmt.Errorf("%s missing from the syscalls of %s", s, argv0)
		}
	}
	return nil
}

// expectProfileSyscalls waits until check accepts the syscalls of the profile of the selector
func expectProfileSyscalls(t *testing.T, test *testModule, selector cgroupModel.WorkloadSelector, check func(profileSyscalls) error) {
	t.Helper()
	var checkErr error
	_, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
		syscalls := make(profileSyscalls)
		for _, node := range walkProfileProcesses(p, func(*activity_tree.ProcessNode) bool { return true }) {
			ids := syscalls[node.Process.Argv0]
			if ids == nil {
				ids = make(map[int]bool)
				syscalls[node.Process.Argv0] = ids
			}
			for id := range node.Syscalls {
				ids[id] = true
			}
		}
		checkErr = check(syscalls)
		return checkErr == nil
	})
	if err != nil {
		if checkErr != nil {
			err = checkErr
		}
		t.Error(err)
	}
}
