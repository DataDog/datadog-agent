// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

var _ = declareInlineConfig(TestSecurityProfileV2SyscallSampling)

// TestSecurityProfileV2SyscallSampling checks that the syscalls sampled for workload profiles v2
// land on the process node that made them, whichever way the kernel reports them: while the
// process runs, when it exits, or when it execs.
func TestSecurityProfileV2SyscallSampling(t *testing.T) {
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
	checkKernelCompatibility(t, "Missing bpf_for_each_map_elem helper", func(kv *kernel.Version) bool {
		return !kv.HasBPFForEachMapElemHelper()
	})

	tagger := NewFakeManualTagger()
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableSecurityProfile:        true,
		securityProfileV2EventTypes:  []string{"exec", "syscalls"},
		eventSamplingSyscallsEnabled: true,
		tagger:                       tagger,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// each subtest gets its own container, and so its own profile
	startWorkload := func(t *testing.T, image string) *dockerCmdWrapper {
		tagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{Image: image, Tag: "latest"})
		dockerInstance, err := test.StartADocker()
		if err != nil {
			t.Fatalf("failed to start a Docker instance: %v", err)
		}
		t.Cleanup(func() { _, _ = dockerInstance.stop() })
		return dockerInstance
	}

	t.Run("running-process", func(t *testing.T) {
		const image = "syscall-sampling-running"
		dockerInstance := startWorkload(t, image)

		// the process still sleeps when its syscalls are checked, so they can only come from the
		// periodic report
		cmd := dockerInstance.Command(syscallTester, []string{"prctl-setname", "sampled", ";", "sleep", "30"}, nil)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()

		if err := test.waitForProfileV2Syscalls(image, 15*time.Second, func(syscalls profileSyscalls) error {
			return syscalls.expect(syscallTester, model.SysPrctl)
		}); err != nil {
			t.Error(err)
		}
	})

	t.Run("exit", func(t *testing.T) {
		const image = "syscall-sampling-exit"
		dockerInstance := startWorkload(t, image)

		// the process exits right after the syscall, before the first periodic report
		if out, err := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, nil).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}

		if err := test.waitForProfileV2Syscalls(image, 10*time.Second, func(syscalls profileSyscalls) error {
			return syscalls.expect(syscallTester, model.SysBind, model.SysExitGroup)
		}); err != nil {
			t.Error(err)
		}
	})

	t.Run("exec", func(t *testing.T) {
		const image = "syscall-sampling-exec"
		dockerInstance := startWorkload(t, image)

		// the process execs right after the syscall, before the first periodic report: the syscall
		// belongs to the node of the process before the exec, not to the executed binary
		if out, err := dockerInstance.Command(syscallTester, []string{"prctl-setname", "sampled", ";", "exec", "/bin/true"}, nil).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}

		if err := test.waitForProfileV2Syscalls(image, 10*time.Second, func(syscalls profileSyscalls) error {
			if err := syscalls.expect(syscallTester, model.SysPrctl, model.SysExecve); err != nil {
				return err
			}
			if err := syscalls.expect("/bin/true", model.SysExitGroup); err != nil {
				return err
			}
			if syscalls["/bin/true"][int(model.SysPrctl)] {
				return errors.New("prctl was reported on the node of the executed binary")
			}
			return nil
		}); err != nil {
			t.Error(err)
		}
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

// waitForProfileV2Syscalls saves the profile of the image until check accepts its syscalls, or the
// timeout expires
func (tm *testModule) waitForProfileV2Syscalls(image string, timeout time.Duration, check func(profileSyscalls) error) error {
	var err error
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		var syscalls profileSyscalls
		if syscalls, err = tm.getProfileV2Syscalls(image); err == nil {
			if err = check(syscalls); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("profile of %s: %w", image, err)
}

func (tm *testModule) getProfileV2Syscalls(image string) (profileSyscalls, error) {
	p, ok := tm.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok {
		return nil, errors.New("not supported")
	}

	msg, err := p.GetProfileManager().SaveSecurityProfile(&api.SecurityProfileSaveParams{
		Selector: &api.WorkloadSelectorMessage{Name: image, Tag: "*"},
	})
	if err != nil {
		return nil, err
	}
	if msg.GetError() != "" {
		return nil, errors.New(msg.GetError())
	}
	defer os.Remove(msg.GetFile())

	prof, err := DecodeSecurityProfile(msg.GetFile())
	if err != nil {
		return nil, err
	}

	syscalls := make(profileSyscalls)
	var walk func(nodes []*activity_tree.ProcessNode)
	walk = func(nodes []*activity_tree.ProcessNode) {
		for _, node := range nodes {
			ids := syscalls[node.Process.Argv0]
			if ids == nil {
				ids = make(map[int]bool)
				syscalls[node.Process.Argv0] = ids
			}
			for id := range node.Syscalls {
				ids[id] = true
			}
			walk(node.Children)
		}
	}
	walk(prof.ActivityTree.ProcessNodes)
	return syscalls, nil
}
