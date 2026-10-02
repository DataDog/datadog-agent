// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

var _ = declareInlineConfig(TestWorkloadProfileContent)

func TestWorkloadProfileContent(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.capabilitiesMonitoringPeriod = time.Second
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range workloadProfileEventTypes() {
		t.Run(et.name, func(t *testing.T) {
			et.skipIfUnsupported(t)

			image := newWorkloadProfileImage()
			dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v1")

			et.trigger(t, dockerInstance, "known")

			p, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return et.hasNode(p, "known")
			})
			if err != nil {
				t.Fatal(err)
			}

			// the profile is shared by all the tags of the image and holds a version context for the tag
			assert.Equal(t, selector, *p.GetWorkloadSelector())
			assert.Equal(t, []string{"v1"}, p.GetVersions())
			versionContext, ok := p.GetVersionContext("v1")
			if assert.True(t, ok, "missing version context for tag v1") {
				assert.Contains(t, versionContext.Tags, "image_name:"+image)
				assert.Contains(t, versionContext.Tags, "image_tag:v1")
			}
			assert.False(t, et.hasNode(p, "new"), "unexpected %s node in the profile", et.name)
		})
	}

	t.Run("capabilities", func(t *testing.T) {
		checkKernelCompatibility(t, "Missing bpf_for_each_map_elem helper", func(kv *kernel.Version) bool {
			return !kv.HasBPFForEachMapElemHelper()
		})

		dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

		execPath := workloadProfileExecPath("capabilities")
		cmd := dockerInstance.Command(execPath, []string{"chroot", "/tmp", ";", "acct"}, []string{})
		_, _ = cmd.CombinedOutput() // ignore error, as the `acct` command is expected to fail

		_, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
			var chroot, pacct bool
			for _, node := range walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
				return strings.HasSuffix(node.Process.FileEvent.PathnameStr, "/wp-exec-capabilities")
			}) {
				for _, capabilityNode := range node.Capabilities {
					switch capabilityNode.Capability {
					case unix.CAP_SYS_CHROOT:
						chroot = chroot || capabilityNode.Capable
					case unix.CAP_SYS_PACCT:
						pacct = pacct || !capabilityNode.Capable
					}
				}
			}
			return chroot && pacct
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

var _ = declareInlineConfig(TestWorkloadProfileDifferentiateArgs)

func TestWorkloadProfileDifferentiateArgs(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.workloadProfileDifferentiateArgs = true
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	execPath := workloadProfileExecPath("args")
	run := func(dockerInstance *dockerCmdWrapper, args ...string) func() error {
		return func() error {
			_, _ = dockerInstance.Command(execPath, args, []string{}).CombinedOutput()
			return nil
		}
	}
	execAnomalyWithArg := func(anomalies []workloadProfileAnomaly, arg string) bool {
		for _, anomaly := range anomalies {
			if anomaly.eventType == model.ExecEventType && strings.Contains(anomaly.json, "wp-exec-args") && strings.Contains(anomaly.json, `"`+arg+`"`) {
				return true
			}
		}
		return false
	}

	dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

	anomalies := collectAnomalies(t, test, func() error {
		_ = run(dockerInstance, "sleep", "1")()
		return run(dockerInstance, "sleep", "2")()
	}, dockerInstance)
	assert.True(t, execAnomalyWithArg(anomalies, "1"), "the first set of arguments should trigger an anomaly")
	assert.True(t, execAnomalyWithArg(anomalies, "2"), "the second set of arguments should trigger an anomaly")

	_, err = waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
		nodes := walkProfileProcesses(p, func(node *activity_tree.ProcessNode) bool {
			return strings.HasSuffix(node.Process.FileEvent.PathnameStr, "/wp-exec-args")
		})
		var args1, args2 bool
		for _, node := range nodes {
			args1 = args1 || slices.Equal(node.Process.Argv, []string{"sleep", "1"})
			args2 = args2 || slices.Equal(node.Process.Argv, []string{"sleep", "2"})
		}
		return len(nodes) == 2 && args1 && args2
	})
	if err != nil {
		t.Fatal(err)
	}

	anomalies = collectAnomalies(t, test, run(dockerInstance, "sleep", "1"), dockerInstance)
	assert.False(t, execAnomalyWithArg(anomalies, "1"), "known arguments shouldn't trigger an anomaly")

	anomalies = collectAnomalies(t, test, run(dockerInstance, "sleep", "3"), dockerInstance)
	assert.True(t, execAnomalyWithArg(anomalies, "3"), "new arguments should trigger an anomaly")
}

var _ = declareInlineConfig(TestWorkloadProfileAnomalyDetection)

func TestWorkloadProfileAnomalyDetection(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(workloadProfileTestOpts(t.TempDir(), tagger)))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range workloadProfileEventTypes() {
		t.Run(et.name, func(t *testing.T) {
			et.skipIfUnsupported(t)

			dockerInstance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "known"), "the first %s activity should trigger an anomaly as soon as the profile starts", et.name)

			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.False(t, et.hasAnomalyFor(anomalies, "known"), "a known %s activity shouldn't trigger an anomaly", et.name)

			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "new")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "new"), "a new %s activity should trigger an anomaly", et.name)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfileWaitFirstPersistence)

func TestWorkloadProfileWaitFirstPersistence(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	const persistencePeriod = 10 * time.Second

	tagger := NewFakeManualTagger()
	storageDir := t.TempDir()
	opts := workloadProfileTestOpts(storageDir, tagger)
	opts.profileReportingWaitPersistence = true
	opts.workloadProfilePersistencePeriod = persistencePeriod
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	m, err := getManagerV2(test)
	if err != nil {
		t.Fatal(err)
	}
	eventTypes := supportedWorkloadProfileEventTypes(t)

	waitForFirstPersistence := func(t *testing.T, image string) {
		t.Helper()
		deadline := time.Now().Add(2 * persistencePeriod)
		for time.Now().Before(deadline) {
			if p := m.GetProfile(cgroupModel.WorkloadSelector{Image: image, Tag: "*"}); p != nil && p.HasAlreadyBeenSent() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("the profile of %s wasn't persisted within %s", image, 2*persistencePeriod)
	}

	// The persistence ticker is shared by all the profiles: wait for a first persistence on a
	// warmup profile so that the profile under test starts right after a tick.
	warmupImage := newWorkloadProfileImage()
	warmupInstance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, warmupImage, "v1")
	_, _ = warmupInstance.Command("true", nil, nil).CombinedOutput()
	waitForFirstPersistence(t, warmupImage)

	// step 1: the profile starts
	image := newWorkloadProfileImage()
	dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v1")

	// step 2: activities go into the profile, but no anomaly is sent until the first persistence
	anomalies := collectAnomalies(t, test, func() error {
		for _, et := range eventTypes {
			et.trigger(t, dockerInstance, "known")
		}
		return nil
	}, dockerInstance)
	if p := m.GetProfile(selector); p == nil {
		t.Fatalf("no profile for %s", image)
	} else if p.HasAlreadyBeenSent() {
		t.Fatalf("the profile was persisted before the end of the first activities, the test raced with the %s persistence ticker", persistencePeriod)
	}
	for _, et := range eventTypes {
		assert.False(t, et.hasAnomalyFor(anomalies, "known"), "no %s anomaly should be sent before the first persistence", et.name)
	}
	snapshot := getWorkloadProfile(t, test, selector)
	for _, et := range eventTypes {
		assert.True(t, et.hasNode(snapshot, "known"), "the %s activity should be in the profile before the first persistence", et.name)
	}

	// step 3: the first persistence of the profile opens the anomaly detection
	waitForFirstPersistence(t, image)
	files, err := os.ReadDir(storageDir)
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, files, "the profile should be persisted in the local storage")

	// step 4: the learned activities don't trigger an anomaly, the new ones do
	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.False(t, et.hasAnomalyFor(anomalies, "known"), "a %s activity learned before the first persistence shouldn't trigger an anomaly", et.name)

			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "new")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "new"), "a new %s activity should trigger an anomaly after the first persistence", et.name)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfileStartupDelay)

func TestWorkloadProfileStartupDelay(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	const startupDelay = 10 * time.Second

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.profilingStartupDelay = startupDelay
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	m, err := getManagerV2(test)
	if err != nil {
		t.Fatal(err)
	}
	var exec workloadProfileEventType
	for _, et := range workloadProfileEventTypes() {
		if et.eventType == model.ExecEventType {
			exec = et
		}
	}
	dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

	// within the startup delay, the activity is ignored: no anomaly, no profile entry
	const ignoredActivityDuration = 3 * time.Second
	if remaining := m.ProfilingStartupDelayRemaining(); remaining < ignoredActivityDuration {
		t.Fatalf("only %s left of the %s profiling startup delay once the module and the container are started", remaining, startupDelay)
	}
	anomalies := collectAnomalies(t, test, func() error {
		exec.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	assert.Positive(t, m.ProfilingStartupDelayRemaining(), "the startup delay ran out while the activity was running")
	assert.False(t, exec.hasAnomalyFor(anomalies, "known"), "no anomaly should be sent during the profiling startup delay")
	if p := getWorkloadProfile(t, test, selector); p != nil {
		assert.False(t, exec.hasNode(p, "known"), "the activity of the profiling startup delay shouldn't be in the profile")
	}

	// once the delay is over, the profile starts and sends anomalies
	time.Sleep(m.ProfilingStartupDelayRemaining() + time.Second)

	anomalies = collectAnomalies(t, test, func() error {
		exec.trigger(t, dockerInstance, "new")
		return nil
	}, dockerInstance)
	assert.True(t, exec.hasAnomalyFor(anomalies, "new"), "a new activity should trigger an anomaly after the profiling startup delay")
	if _, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
		return exec.hasNode(p, "new")
	}); err != nil {
		t.Error(err)
	}

	anomalies = collectAnomalies(t, test, func() error {
		exec.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	assert.True(t, exec.hasAnomalyFor(anomalies, "known"), "the activity of the profiling startup delay wasn't learned, it should trigger an anomaly")
}

var _ = declareInlineConfig(TestWorkloadProfileAnomalyDetectionVariables)

func TestWorkloadProfileAnomalyDetectionVariables(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	// When both the rule and the anomaly detection fire on the same event, the variable set by the
	// rule should be present in the anomaly detection event.
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_wp_variable_rule",
			Expression: `exec.file.name == "wp-exec-new"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:  "wp_test_var",
						Value: true,
					},
				},
			},
		},
	}

	tagger := NewFakeManualTagger()
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(workloadProfileTestOpts(t.TempDir(), tagger)))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	var exec workloadProfileEventType
	for _, et := range workloadProfileEventTypes() {
		if et.eventType == model.ExecEventType {
			exec = et
		}
	}

	dockerInstance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")
	exec.trigger(t, dockerInstance, "known")

	anomalies := collectAnomalies(t, test, func() error {
		exec.trigger(t, dockerInstance, "new")
		return nil
	}, dockerInstance)

	var found bool
	for _, anomaly := range anomalies {
		if anomaly.eventType == model.ExecEventType && exec.matchAnomaly(anomaly, "new") {
			found = true
			assert.Contains(t, anomaly.json, `"wp_test_var"`, "the anomaly detection event should contain the variable set by the rule")
		}
	}
	assert.True(t, found, "the new exec should trigger an anomaly")
}

var _ = declareInlineConfig(TestWorkloadProfileImageTags)

func TestWorkloadProfileImageTags(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(workloadProfileTestOpts(t.TempDir(), tagger)))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range workloadProfileEventTypes() {
		t.Run(et.name, func(t *testing.T) {
			et.skipIfUnsupported(t)

			image := newWorkloadProfileImage()
			v1Instance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v1")
			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, v1Instance, "known")
				return nil
			}, v1Instance)
			assert.True(t, et.hasAnomalyFor(anomalies, "known"), "the first %s activity of the image should trigger an anomaly", et.name)

			// a second tag of the same image shares the profile: what the first tag learned is known
			v2Instance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v2")
			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, v2Instance, "known")
				return nil
			}, v2Instance)
			assert.False(t, et.hasAnomalyFor(anomalies, "known"), "a %s activity learned under another tag shouldn't trigger an anomaly", et.name)

			p, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				versions := p.GetVersions()
				return slices.Contains(versions, "v1") && slices.Contains(versions, "v2")
			})
			if err != nil {
				t.Fatal(err)
			}
			assert.True(t, et.hasNode(p, "known"), "the %s activity should be in the profile", et.name)

			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, v2Instance, "new")
				return nil
			}, v2Instance)
			assert.True(t, et.hasAnomalyFor(anomalies, "new"), "a new %s activity should trigger an anomaly under the second tag", et.name)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfileSyscalls)

func TestWorkloadProfileSyscalls(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.eventSamplingSyscallsEnabled = true
	opts.workloadProfileEventTypes = []string{"exec", "open", "dns", "bind", "connect", "capabilities", "syscalls"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	execPath := workloadProfileExecPath("syscalls")
	run := func(dockerInstance *dockerCmdWrapper, args ...string) func() error {
		return func() error {
			_, _ = dockerInstance.Command(execPath, args, []string{}).CombinedOutput()
			return nil
		}
	}
	syscallAnomalies := func(anomalies []workloadProfileAnomaly) []workloadProfileAnomaly {
		var out []workloadProfileAnomaly
		for _, anomaly := range anomalies {
			if anomaly.eventType == model.SyscallsEventType && strings.Contains(anomaly.json, "wp-exec-syscalls") {
				out = append(out, anomaly)
			}
		}
		return out
	}

	dockerInstance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

	anomalies := collectAnomalies(t, test, run(dockerInstance, "sleep", "1"), dockerInstance)
	assert.NotEmpty(t, syscallAnomalies(anomalies), "the syscalls of the first run should trigger anomalies")

	anomalies = collectAnomalies(t, test, run(dockerInstance, "sleep", "1"), dockerInstance)
	assert.Empty(t, syscallAnomalies(anomalies), "known syscalls shouldn't trigger an anomaly")

	anomalies = collectAnomalies(t, test, run(dockerInstance, "chroot", "/tmp"), dockerInstance)
	var chroot bool
	for _, anomaly := range syscallAnomalies(anomalies) {
		chroot = chroot || strings.Contains(anomaly.json, `"chroot"`)
	}
	assert.True(t, chroot, "a new syscall should trigger an anomaly: %+v", anomalies)
}

var _ = declareInlineConfig(TestWorkloadProfileNodeEviction)

func TestWorkloadProfileNodeEviction(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	const evictionTimeout = 5 * time.Second

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.securityProfileNodeEvictionTimeout = evictionTimeout
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	m, err := getManagerV2(test)
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range workloadProfileEventTypes() {
		t.Run(et.name, func(t *testing.T) {
			et.skipIfUnsupported(t)

			dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

			et.trigger(t, dockerInstance, "known")
			if _, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return et.hasNode(p, "known")
			}); err != nil {
				t.Fatal(err)
			}

			time.Sleep(evictionTimeout + time.Second)
			m.EvictUnusedNodes()

			p := getWorkloadProfile(t, test, selector)
			if p == nil {
				t.Fatal("the profile shouldn't be removed by the node eviction")
			}
			assert.False(t, et.hasNode(p, "known"), "the inactive %s activity should be evicted", et.name)

			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "known"), "an evicted %s activity should trigger an anomaly again", et.name)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfilePersistence)

func TestWorkloadProfilePersistence(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	storageDir := t.TempDir()
	image := newWorkloadProfileImage()

	// first run: learn an activity of each type, then persist the profile
	tagger := NewFakeManualTagger()
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(workloadProfileTestOpts(storageDir, tagger)))
	if err != nil {
		t.Fatal(err)
	}
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		test.CloseTest()
		t.Fatal(err)
	}
	supported := supportedWorkloadProfileEventTypes(t)

	dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v1")
	for _, et := range supported {
		et.trigger(t, dockerInstance, "known")
	}
	if _, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
		for _, et := range supported {
			if !et.hasNode(p, "known") {
				return false
			}
		}
		return true
	}); err != nil {
		test.CloseTest()
		t.Fatal(err)
	}
	m, err := getManagerV2(test)
	if err != nil {
		test.CloseTest()
		t.Fatal(err)
	}
	m.PersistAllProfiles()
	_, _ = dockerInstance.stop()
	test.CloseTest()

	// second run: the profile is reloaded from the local storage for a new container of the image
	tagger = NewFakeManualTagger()
	test, err = newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(workloadProfileTestOpts(storageDir, tagger)), withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()
	syscallTester, err = loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstance, _ = startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v1")
	for _, et := range supportedWorkloadProfileEventTypes(t) {
		t.Run(et.name, func(t *testing.T) {
			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.False(t, et.hasAnomalyFor(anomalies, "known"), "a %s activity learned before the restart shouldn't trigger an anomaly", et.name)

			p, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return et.hasNode(p, "known")
			})
			if err != nil {
				t.Fatal(err)
			}
			assert.True(t, et.hasNode(p, "known"), "the reloaded profile should contain the %s activity", et.name)

			anomalies = collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "new")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "new"), "a new %s activity should trigger an anomaly after the restart", et.name)
		})
	}
}
