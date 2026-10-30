// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// The tests of this file only run in the KMT job dedicated to the workload profiles (test set cws_wprofiles), the
// others are skipped by skipIfNoWorkloadProfileEnv. The TestWorkloadProfileDedicated prefix is the one selected by
// cws_wprofiles.json: the workload profile tests that run in every job are in workload_profile_test.go.

// Profile content

var _ = declareInlineConfig(TestWorkloadProfileDedicatedContent)

// TestWorkloadProfileDedicatedContent checks, for each event type, the content of the profile (selector, version context,
// learned activity), and the capabilities learned for a process.
func TestWorkloadProfileDedicatedContent(t *testing.T) {
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedSyscalls)

// TestWorkloadProfileDedicatedSyscalls checks that the syscalls of a process are learned, and that a new syscall triggers
// an anomaly.
func TestWorkloadProfileDedicatedSyscalls(t *testing.T) {
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
	assert.NotEmpty(t, syscallAnomalies(anomalies), "a new process is a new syscall sample, so known syscalls are emitted again while sampling is on")

	anomalies = collectAnomalies(t, test, run(dockerInstance, "chroot", "/tmp"), dockerInstance)
	var chroot bool
	for _, anomaly := range syscallAnomalies(anomalies) {
		chroot = chroot || strings.Contains(anomaly.json, `"chroot"`)
	}
	assert.True(t, chroot, "a new syscall should trigger an anomaly: %+v", anomalies)
}

var _ = declareInlineConfig(TestWorkloadProfileDedicatedDifferentiateArgs)

// TestWorkloadProfileDedicatedDifferentiateArgs checks that, when enabled, the same executable run with different arguments
// gets one process node per set of arguments.
func TestWorkloadProfileDedicatedDifferentiateArgs(t *testing.T) {
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedOpenSampleKey)

// TestWorkloadProfileDedicatedOpenSampleKey checks that every process node gets its own open, even when the processes
// share the kernel open sample key. That key is (ppid, executable, file, comm): run by the same shell,
// `cat /etc/hosts` and `cat -n /etc/hosts` share it, although their arguments make them two process nodes.
// Run by two shells, they have two keys.
func TestWorkloadProfileDedicatedOpenSampleKey(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.workloadProfileDifferentiateArgs = true
	// sample_pass must not reopen the cookie of the first open while the test runs
	opts.anomalyDetectionEventCooldown = 10 * time.Minute
	// The policy file outlives the test module, and an empty rule list does not rewrite it.
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_wp_open_sample_key_unused",
		Expression: `open.file.path == "/tmp/wp-open-sample-key-unused"`,
	}}
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	const hosts = "/etc/hosts"
	catArgs := []string{hosts}
	catNArgs := []string{"-n", hosts}

	hasAnomaly := func(anomalies []workloadProfileAnomaly, eventType model.EventType, args []string, path string) bool {
		return hasAnomalyProcess(t, anomalies, eventType, func(p workloadProfileAnomalyProcess) bool {
			return slices.Equal(p.args, args) && (path == "" || p.path == path)
		})
	}

	cases := []struct {
		name string
		// each script runs in its own shell, the parent of the processes it starts
		scripts []string
	}{
		{
			name: "same parent",
			// the sleep gives userspace the time to send the first anomaly and block its cookie.
			// The trailing builtin keeps the shell from exec'ing the last cat in place of itself.
			scripts: []string{"cat /etc/hosts; sleep 3; cat -n /etc/hosts; true"},
		},
		{
			name:    "different parents",
			scripts: []string{"cat /etc/hosts; true", "sleep 3; cat -n /etc/hosts; true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

			anomalies := collectAnomalies(t, test, func() error {
				for _, script := range tc.scripts {
					if out, err := dockerInstance.Command("sh", []string{"-c", script}, []string{}).CombinedOutput(); err != nil {
						t.Logf("sh -c %q: %v (%s)", script, err, out)
					}
				}
				return nil
			}, dockerInstance)

			require.True(t, hasAnomaly(anomalies, model.ExecEventType, catArgs, ""), "cat %s should be a new process node", hosts)
			require.True(t, hasAnomaly(anomalies, model.ExecEventType, catNArgs, ""), "cat -n %s should be a new process node", hosts)
			require.True(t, hasAnomaly(anomalies, model.FileOpenEventType, catArgs, hosts), "the open of cat %s should trigger an anomaly", hosts)
			assert.True(t, hasAnomaly(anomalies, model.FileOpenEventType, catNArgs, hosts), "the open of cat -n %s is a new file node of another process node, it should trigger an anomaly", hosts)

			_, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return profileHasFileOpenedWithArgs(p, catArgs, "hosts") && profileHasFileOpenedWithArgs(p, catNArgs, "hosts")
			})
			assert.NoError(t, err, "both process nodes should hold their open of %s", hosts)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfileDedicatedOpenSampleKeyHardlink)

// TestWorkloadProfileDedicatedOpenSampleKeyHardlink checks that two hardlinks of one executable, run by the same shell
// with the same comm, each get their own open, with the default configuration. Process nodes are keyed by path, so the
// links are two process nodes, while the kernel open sample key holds the inode of the executable. The kernel bumps
// the path id of a file with several links on every lookup, which gives each exec its own key.
func TestWorkloadProfileDedicatedOpenSampleKeyHardlink(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	// sample_pass must not reopen the cookie of the first open while the test runs
	opts.anomalyDetectionEventCooldown = 10 * time.Minute
	// The policy file outlives the test module, and an empty rule list does not rewrite it.
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_wp_open_sample_key_unused",
		Expression: `open.file.path == "/tmp/wp-open-sample-key-unused"`,
	}}
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstance, selector := startWorkloadProfileContainer(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1")

	// busybox picks the applet from the basename, so both links run cat, with the comm cat
	const (
		hosts = "/etc/hosts"
		linkA = "/tmp/wp-link-a/cat"
		linkB = "/tmp/wp-link-b/cat"
	)
	setup := "mkdir -p /tmp/wp-link-a /tmp/wp-link-b && cp /bin/busybox " + linkA + " && ln " + linkA + " " + linkB
	if out, err := dockerInstance.Command("sh", []string{"-c", setup}, []string{}).CombinedOutput(); err != nil {
		t.Fatalf("couldn't create the hardlinks: %v (%s)", err, out)
	}

	// the sleep gives userspace the time to send the first anomaly and block its cookie.
	// The trailing builtin keeps the shell from exec'ing the last cat in place of itself.
	script := linkA + " " + hosts + "; sleep 3; " + linkB + " " + hosts + "; true"
	anomalies := collectAnomalies(t, test, func() error {
		if out, err := dockerInstance.Command("sh", []string{"-c", script}, []string{}).CombinedOutput(); err != nil {
			t.Logf("sh -c %q: %v (%s)", script, err, out)
		}
		return nil
	}, dockerInstance)

	hasAnomaly := func(eventType model.EventType, executable string, path string) bool {
		return hasAnomalyProcess(t, anomalies, eventType, func(p workloadProfileAnomalyProcess) bool {
			return p.executable == executable && (path == "" || p.path == path)
		})
	}
	require.True(t, hasAnomaly(model.ExecEventType, linkA, ""), "%s should be a new process node", linkA)
	require.True(t, hasAnomaly(model.ExecEventType, linkB, ""), "%s should be a new process node", linkB)
	require.True(t, hasAnomaly(model.FileOpenEventType, linkA, hosts), "the open of %s by %s should trigger an anomaly", hosts, linkA)
	assert.True(t, hasAnomaly(model.FileOpenEventType, linkB, hosts), "the open of %s by %s is a new file node of another process node, it should trigger an anomaly", hosts, linkB)

	openedBy := func(executable string) func(node *activity_tree.ProcessNode) bool {
		return func(node *activity_tree.ProcessNode) bool {
			return node.Process.FileEvent.PathnameStr == executable
		}
	}
	_, err = waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
		return profileHasFileOpenedByProcess(p, openedBy(linkA), "hosts") && profileHasFileOpenedByProcess(p, openedBy(linkB), "hosts")
	})
	assert.NoError(t, err, "both process nodes should hold their open of %s", hosts)
}

// Anomaly detection

var _ = declareInlineConfig(TestWorkloadProfileDedicatedAnomalyDetection)

// TestWorkloadProfileDedicatedAnomalyDetection checks, for each event type, that the first activity and a new activity
// trigger an anomaly, and that a known activity doesn't.
func TestWorkloadProfileDedicatedAnomalyDetection(t *testing.T) {
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedEventCooldown)

const (
	// cooldownSampling is sampling on and no matching rule: the kernel sampler admits the activity with a cookie.
	// sample_pass, which userspace manages, gates it in the kernel, and userspace emits whatever it delivers.
	cooldownSampling = "sampling"
	// cooldownSamplingRule is sampling on, and a rule that approves the activity is loaded after its first sampled
	// emit. The approver runs before the sampler, so the next events carry no cookie: they always reach userspace,
	// and the activity node cooldown decides. When the node emits again, the cookie the sampler gave it is blocked
	// again.
	cooldownSamplingRule = "sampling-rule"
	// cooldownNoSampling is sampling off. Filters are off too, so the events still arrive, without a cookie. As for
	// the event types that are never sampled (exec for instance), the activity node cooldown decides.
	cooldownNoSampling = "no-sampling"
)

// TestWorkloadProfileDedicatedEventCooldown checks, for every event type that can produce it, the cooldown gate of
// each sampling mode.
func TestWorkloadProfileDedicatedEventCooldown(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	for _, mode := range []string{cooldownSampling, cooldownSamplingRule, cooldownNoSampling} {
		t.Run(mode, func(t *testing.T) {
			runWorkloadProfileCooldownMode(t, mode)
		})
	}
}

func runWorkloadProfileCooldownMode(t *testing.T, mode string) {
	t.Helper()

	// The no-sampling mode waits this out. The other modes must still be inside it after several docker execs
	// (and a policy reload), so a repeat that is emitted, or held, cannot be explained by the node cooldown having
	// elapsed.
	eventCooldown := 5 * time.Second
	switch mode {
	case cooldownSampling:
		eventCooldown = 45 * time.Second
	case cooldownSamplingRule:
		eventCooldown = 20 * time.Second
	}

	tagger := NewFakeManualTagger()
	opts := workloadProfileTestOpts(t.TempDir(), tagger)
	opts.anomalyDetectionEventCooldown = eventCooldown
	opts.capabilitiesMonitoringEnabled = true
	opts.capabilitiesMonitoringPeriod = time.Second
	opts.workloadProfileEventTypes = []string{"exec", "open", "dns", "bind", "connect", "capabilities", "mount", "syscalls"}
	if mode == cooldownNoSampling {
		opts.disableEventSampling = true
		opts.disableFilters = true
	} else {
		opts.eventSamplingSyscallsEnabled = true
	}

	// The policy file outlives the test module, and an empty rule list does not rewrite it. Always write one, so
	// the rules of a previous test cannot approve the activity.
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_wp_cooldown_unused",
		Expression: `open.file.path == "/tmp/wp-cooldown-unused"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	for _, et := range workloadProfileEventCooldownTypes() {
		t.Run(et.name, func(t *testing.T) {
			switch mode {
			case cooldownSampling:
				if !eventTypeHasSampleCookie(et.eventType) {
					t.Skip("this event type is never sampled, the no-sampling mode covers it")
				}
			case cooldownSamplingRule:
				if !eventTypeHasSampleCookie(et.eventType) || workloadProfileCooldownRule(et) == nil {
					t.Skip("needs both a sampled event type and a rule that can approve the activity")
				}
			case cooldownNoSampling:
				if et.eventType == model.SyscallsEventType {
					t.Skip("the kernel delivers syscall events only through their sampler; the node cooldown is covered by TestManagerV2_eventCooldownGate")
				}
			}
			et.skipIfUnsupported(t)

			dockerInstance, _ := startWorkloadProfileContainerWithRunArgs(t, test, tagger, syscallTester, newWorkloadProfileImage(), "v1", et.containerRunArgs)
			started := time.Now()

			switch mode {
			case cooldownSampling:
				if cooldownKernelAdmitsNewProcess(et) {
					runFreshSampleCookie(t, test, et, dockerInstance, started, eventCooldown)
				} else {
					runStableSampleCookie(t, test, et, dockerInstance, started, eventCooldown)
				}
			case cooldownSamplingRule:
				ruleDefs = append(ruleDefs, workloadProfileCooldownRule(et))
				runSamplingRule(t, test, et, dockerInstance, ruleDefs, eventCooldown)
			default:
				runNodeCooldown(t, test, et, dockerInstance, eventCooldown)
			}
		})
	}
}

// runNodeCooldown is the cookie-0 gate: the repeat stays silent until the activity node cooldown elapses.
func runNodeCooldown(t *testing.T, test *testModule, et workloadProfileEventType, dockerInstance *dockerCmdWrapper, eventCooldown time.Duration) {
	t.Helper()

	first := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.True(t, et.hasAnomalyFor(first, "known"), "the first %s activity should trigger an anomaly", et.name)

	second := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.False(t, et.hasAnomalyFor(second, "known"), "cookie 0: this %s repeat stays on the activity node cooldown", et.name)

	time.Sleep(eventCooldown + time.Second)

	third := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.True(t, et.hasAnomalyFor(third, "known"), "the same %s activity should trigger an anomaly again once the event cooldown has elapsed", et.name)
}

// runFreshSampleCookie covers bind, connect and syscalls. A new process is a new sample key, so the
// kernel mints a cookie that sample_pass does not know yet. A missing entry allows the event even
// though the activity node cooldown has not elapsed. The cookie of the first emit stays blocked.
func runFreshSampleCookie(t *testing.T, test *testModule, et workloadProfileEventType, dockerInstance *dockerCmdWrapper, started time.Time, eventCooldown time.Duration) {
	t.Helper()

	before := samplePassSnapshot(t, test)

	first := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.True(t, et.hasAnomalyFor(first, "known"), "the first %s activity should trigger an anomaly", et.name)
	blocked := newBlockedCookies(t, test, before)

	second := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	assertInsideNodeCooldown(t, started, eventCooldown)
	require.True(t, et.hasAnomalyFor(second, "known"), "a new %s process is a new cookie, and a missing sample_pass entry lets it through inside the node cooldown", et.name)
	assertSamplePass(t, test, blocked, 0)
}

// runStableSampleCookie covers open. The sample key does not include the pid, so both processes share
// one cookie. 0 holds it. Writing 1 reopens it before the node cooldown has elapsed.
func runStableSampleCookie(t *testing.T, test *testModule, et workloadProfileEventType, dockerInstance *dockerCmdWrapper, started time.Time, eventCooldown time.Duration) {
	t.Helper()

	before := samplePassSnapshot(t, test)

	first := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.True(t, et.hasAnomalyFor(first, "known"), "the first %s activity should trigger an anomaly", et.name)
	blocked := newBlockedCookies(t, test, before)

	held := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	require.False(t, et.hasAnomalyFor(held, "known"), "sample_pass is 0, so this %s repeat is held", et.name)
	assertSamplePass(t, test, blocked, 0)

	setSamplePass(t, test, blocked, 1)

	reopened := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	assertInsideNodeCooldown(t, started, eventCooldown)
	require.True(t, et.hasAnomalyFor(reopened, "known"), "sample_pass set to 1 lets this %s cookie through before the node cooldown has elapsed", et.name)

	heldAgain := collectAnomalies(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance)
	assertInsideNodeCooldown(t, started, eventCooldown)
	require.False(t, et.hasAnomalyFor(heldAgain, "known"), "the emit wrote sample_pass back to 0, so the next %s repeat is held again", et.name)
}

// runSamplingRule starts with the sampler admitting the activity, then loads ruleDefs, whose last rule approves it.
func runSamplingRule(t *testing.T, test *testModule, et workloadProfileEventType, dockerInstance *dockerCmdWrapper, ruleDefs []*rules.RuleDefinition, eventCooldown time.Duration) {
	t.Helper()

	first, firstCookies := collectCooldownActivity(t, test, et, dockerInstance)
	firstSent := time.Now()
	require.True(t, et.hasAnomalyFor(first, "known"), "the first %s activity should trigger an anomaly", et.name)
	nodeCookies := slices.DeleteFunc(firstCookies, func(cookie uint64) bool { return cookie == 0 })
	require.NotEmpty(t, nodeCookies, "without a rule, the %s activity goes through the sampler", et.name)
	assertSamplePass(t, test, nodeCookies, 0)

	require.NoError(t, setTestPolicy(commonCfgDir, nil, ruleDefs))
	require.NoError(t, test.reloadPolicies())

	held, heldCookies := collectCooldownActivity(t, test, et, dockerInstance)
	assertInsideNodeCooldown(t, firstSent, eventCooldown)
	require.NotEmpty(t, heldCookies, "the approved %s activity should reach userspace", et.name)
	require.Equal(t, make([]uint64, len(heldCookies)), heldCookies, "the rule approves the %s activity before the sampler runs, so it carries no cookie", et.name)
	require.False(t, et.hasAnomalyFor(held, "known"), "this %s repeat stays on the node cooldown started by the sampled emit", et.name)

	time.Sleep(time.Until(firstSent.Add(eventCooldown + time.Second)))
	assertSamplePass(t, test, nodeCookies, 1)

	again, _ := collectCooldownActivity(t, test, et, dockerInstance)
	require.True(t, et.hasAnomalyFor(again, "known"), "the same %s activity should trigger an anomaly again once the node cooldown has elapsed", et.name)
	assertSamplePass(t, test, nodeCookies, 0)
}

// collectCooldownActivity triggers the "known" activity of et, and returns the anomalies, and the sample cookie of
// every event of that activity.
func collectCooldownActivity(t *testing.T, test *testModule, et workloadProfileEventType, dockerInstance *dockerCmdWrapper) ([]workloadProfileAnomaly, []uint64) {
	t.Helper()

	var (
		lock    sync.Mutex
		cookies []uint64
	)
	anomalies := collectAnomaliesWithProbeEvents(t, test, func() error {
		et.trigger(t, dockerInstance, "known")
		return nil
	}, dockerInstance, func(event *model.Event) {
		if !isCooldownActivity(et, event) {
			return
		}
		lock.Lock()
		cookies = append(cookies, eventSampleCookie(event))
		lock.Unlock()
	})

	lock.Lock()
	defer lock.Unlock()
	return anomalies, slices.Clone(cookies)
}

// isCooldownActivity reports whether event is the "known" activity of et
func isCooldownActivity(et workloadProfileEventType, event *model.Event) bool {
	if event.GetEventType() != et.eventType {
		return false
	}
	switch et.eventType {
	case model.FileOpenEventType:
		path, _ := event.GetFieldValue("open.file.path")
		return path == "/tmp/wp-open-known"
	case model.BindEventType:
		port, _ := event.GetFieldValue("bind.addr.port")
		return port == workloadProfilePorts["known"]
	case model.ConnectEventType:
		port, _ := event.GetFieldValue("connect.addr.port")
		return port == workloadProfilePorts["known"]
	default:
		return false
	}
}

// eventSampleCookie returns the kernel sample cookie of event, 0 if it has none
func eventSampleCookie(event *model.Event) uint64 {
	switch event.GetEventType() {
	case model.FileOpenEventType:
		return event.Open.SampleCookie
	case model.BindEventType:
		return event.Bind.SampleCookie
	case model.ConnectEventType:
		return event.Connect.SampleCookie
	case model.SyscallsEventType:
		return event.Syscalls.SampleCookie
	default:
		return 0
	}
}

func assertInsideNodeCooldown(t *testing.T, started time.Time, eventCooldown time.Duration) {
	t.Helper()
	require.Less(t, time.Since(started), eventCooldown, "the activity node cooldown must still be running")
}

func samplePassMap(t *testing.T, test *testModule) *ebpf.Map {
	t.Helper()
	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	require.True(t, ok, "probe type isn't EBPF")
	m, _, err := p.Manager.Get().GetMap("sample_pass")
	require.NoError(t, err)
	require.NotNil(t, m)
	return m
}

func samplePassSnapshot(t *testing.T, test *testModule) map[uint64]uint8 {
	t.Helper()
	entries := make(map[uint64]uint8)
	var key uint64
	var value uint8
	it := samplePassMap(t, test).Iterate()
	for it.Next(&key, &value) {
		entries[key] = value
	}
	require.NoError(t, it.Err())
	return entries
}

// newBlockedCookies returns the sample_pass keys written since before. An emit blocks its cookie with 0.
func newBlockedCookies(t *testing.T, test *testModule, before map[uint64]uint8) []uint64 {
	t.Helper()
	after := samplePassSnapshot(t, test)
	var cookies []uint64
	for key, value := range after {
		if _, seen := before[key]; seen {
			continue
		}
		require.Equal(t, uint8(0), value, "sample_pass[%d] should be 0 after the anomaly was sent", key)
		cookies = append(cookies, key)
	}
	require.NotEmpty(t, cookies, "the anomaly should have blocked its sample cookie")
	return cookies
}

func setSamplePass(t *testing.T, test *testModule, cookies []uint64, value uint8) {
	t.Helper()
	m := samplePassMap(t, test)
	for _, cookie := range cookies {
		require.NoError(t, m.Put(cookie, value))
	}
}

func assertSamplePass(t *testing.T, test *testModule, cookies []uint64, want uint8) {
	t.Helper()
	m := samplePassMap(t, test)
	for _, cookie := range cookies {
		var got uint8
		require.NoError(t, m.Lookup(cookie, &got), "sample_pass[%d]", cookie)
		require.Equal(t, want, got, "sample_pass[%d]", cookie)
	}
}

// eventTypeHasSampleCookie reports whether the kernel can attach a sample cookie to this event type.
func eventTypeHasSampleCookie(eventType model.EventType) bool {
	switch eventType {
	case model.FileOpenEventType, model.BindEventType, model.ConnectEventType, model.SyscallsEventType:
		return true
	default:
		return false
	}
}

// cooldownKernelAdmitsNewProcess reports whether a second process of the same activity is a new
// kernel sample. The new cookie is absent from sample_pass, so the repeat is emitted.
func cooldownKernelAdmitsNewProcess(et workloadProfileEventType) bool {
	switch et.eventType {
	case model.BindEventType, model.ConnectEventType, model.SyscallsEventType:
		return true
	default:
		return false
	}
}

// workloadProfileCooldownRule is the rule that approves the "known" activity of et.
// nil means the event type cannot be matched by a SECL rule.
func workloadProfileCooldownRule(et workloadProfileEventType) *rules.RuleDefinition {
	var expression string
	switch et.name {
	case "open":
		expression = `open.file.path == "/tmp/wp-open-known"`
	case "exec":
		expression = `exec.file.name == "wp-exec-known"`
	case "bind":
		expression = `bind.addr.family == AF_INET && bind.addr.port == 4251`
	case "connect":
		expression = `connect.addr.family == AF_INET && connect.addr.port == 4251`
	case "dns":
		expression = `dns.question.name == "one.one.one.one"`
	case "capabilities":
		expression = `capabilities.used == CAP_SYS_CHROOT && process.file.name == "wp-exec-capabilities"`
	case "mount":
		expression = `mount.mountpoint.path == "/tmp/wp-mount-dst-known"`
	default:
		return nil
	}
	return &rules.RuleDefinition{ID: "test_wp_cooldown_" + et.name, Expression: expression}
}

var _ = declareInlineConfig(TestWorkloadProfileDedicatedAnomalyDetectionVariables)

// TestWorkloadProfileDedicatedAnomalyDetectionVariables checks that the variables set by a rule are present in the anomaly
// detection event raised on the same event.
func TestWorkloadProfileDedicatedAnomalyDetectionVariables(t *testing.T) {
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

// Image tags

var _ = declareInlineConfig(TestWorkloadProfileDedicatedImageTags)

// TestWorkloadProfileDedicatedImageTags checks, for each event type, that the tags of an image share one profile: what one
// tag learned is known to the others.
func TestWorkloadProfileDedicatedImageTags(t *testing.T) {
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedVersionContextTags)

// TestWorkloadProfileDedicatedVersionContextTags checks that the version context of each image tag holds its own image tag,
// and no container of another tag.
func TestWorkloadProfileDedicatedVersionContextTags(t *testing.T) {
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

	v1Instance, v2Instance, p := startTwoImageTagsWorkload(t, test, tagger, syscallTester)

	for _, tc := range []struct {
		tag      string
		instance *dockerCmdWrapper
		other    *dockerCmdWrapper
	}{
		{tag: "v1", instance: v1Instance, other: v2Instance},
		{tag: "v2", instance: v2Instance, other: v1Instance},
	} {
		versionContext, ok := p.GetVersionContext(tc.tag)
		if !assert.True(t, ok, "missing version context for tag %s", tc.tag) {
			continue
		}
		assert.Equal(t, []string{tc.tag}, tagValues(versionContext.Tags, "image_tag"), "the version context of tag %s should hold its own image tag", tc.tag)
		assert.NotContains(t, versionContext.Tags, "container_id:"+tc.other.containerID, "the version context of tag %s shouldn't hold the container id of a container of another tag", tc.tag)
	}
}

var _ = declareInlineConfig(TestWorkloadProfileDedicatedAnomalyTags)

// TestWorkloadProfileDedicatedAnomalyTags checks that the anomaly of a container holds the tags of its own image tag and
// container, not the ones of another tag.
func TestWorkloadProfileDedicatedAnomalyTags(t *testing.T) {
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
			et.trigger(t, v1Instance, "known")
			if _, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return et.hasNode(p, "known")
			}); err != nil {
				t.Fatal(err)
			}

			v2Instance, _ := startWorkloadProfileContainer(t, test, tagger, syscallTester, image, "v2")
			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, v2Instance, "new")
				return nil
			}, v2Instance)

			var found bool
			for _, anomaly := range anomalies {
				if anomaly.eventType != et.eventType || !et.matchAnomaly(anomaly, "new") {
					continue
				}
				found = true
				tags := anomalySecurityProfileTags(t, anomaly)
				assert.Equal(t, []string{"v2"}, tagValues(tags, "image_tag"), "the %s anomaly of a v2 container should hold the v2 image tag", et.name)
				assert.NotContains(t, tags, "container_id:"+v1Instance.containerID, "the %s anomaly of a v2 container shouldn't hold the container id of the v1 container", et.name)
			}
			assert.True(t, found, "the new %s activity of the v2 container should trigger an anomaly", et.name)
		})
	}
}

var _ = declareInlineConfig(TestWorkloadProfileDedicatedTags)

// TestWorkloadProfileDedicatedTags checks that the tags sent with a profile shared by several image tags and containers
// cover all of them.
func TestWorkloadProfileDedicatedTags(t *testing.T) {
	t.Skip("Skip until the fix is implemented")

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

	v1Instance, v2Instance, p := startTwoImageTagsWorkload(t, test, tagger, syscallTester)

	m, err := getManagerV2(test)
	if err != nil {
		t.Fatal(err)
	}
	// the profile format doesn't hold the tags of the profile, the live profile does and they are sent with it to the backend
	live := m.GetProfile(*p.GetWorkloadSelector())
	if live == nil {
		t.Fatal("no live profile")
	}
	tags := live.GetTags()
	if !assert.NotEmpty(t, tags, "the live profile should hold tags") {
		return
	}
	if imageTags := tagValues(tags, "image_tag"); len(imageTags) > 0 {
		assert.ElementsMatch(t, []string{"v1", "v2"}, imageTags, "the tags of a profile shared by several image tags shouldn't only hold the image tag of its first container")
	}
	if containerIDs := tagValues(tags, "container_id"); len(containerIDs) > 0 {
		assert.ElementsMatch(t, []string{v1Instance.containerID, v2Instance.containerID}, containerIDs, "the tags of a profile shared by several containers shouldn't only hold the container id of its first container")
	}
}

// Profile lifecycle

var _ = declareInlineConfig(TestWorkloadProfileDedicatedStartupDelay)

// TestWorkloadProfileDedicatedStartupDelay checks that the activities within the profiling startup delay are ignored, and
// that the profile starts once the delay is over.
func TestWorkloadProfileDedicatedStartupDelay(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	const startupDelay = 30 * time.Second

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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedWaitFirstPersistence)

// TestWorkloadProfileDedicatedWaitFirstPersistence checks that, when waiting for the first persistence, the activities are
// learned but no anomaly is sent until the profile has been persisted once.
func TestWorkloadProfileDedicatedWaitFirstPersistence(t *testing.T) {
	skipIfNoWorkloadProfileEnv(t)

	tagger := NewFakeManualTagger()
	storageDir := t.TempDir()
	opts := workloadProfileTestOpts(storageDir, tagger)
	opts.profileReportingWaitPersistence = true
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
		t.Fatal("the profile shouldn't have been persisted before the explicit persistence")
	}
	for _, et := range eventTypes {
		assert.False(t, et.hasAnomalyFor(anomalies, "known"), "no %s anomaly should be sent before the first persistence", et.name)
	}
	snapshot := getWorkloadProfile(t, test, selector)
	for _, et := range eventTypes {
		assert.True(t, et.hasNode(snapshot, "known"), "the %s activity should be in the profile before the first persistence", et.name)
	}

	// step 3: the first persistence of the profile opens the anomaly detection
	m.PersistAllProfiles()
	if p := m.GetProfile(selector); p == nil || !p.HasAlreadyBeenSent() {
		t.Fatalf("the profile of %s should have been marked as sent after its persistence", image)
	}
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedPersistence)

// TestWorkloadProfileDedicatedPersistence checks that a persisted profile is reloaded after a restart, and that the
// activities it holds are known.
func TestWorkloadProfileDedicatedPersistence(t *testing.T) {
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

var _ = declareInlineConfig(TestWorkloadProfileDedicatedNodeEviction)

// TestWorkloadProfileDedicatedNodeEviction checks that the inactive nodes are evicted from the profile while the node of a
// running process is kept, and that an evicted activity triggers an anomaly again.
func TestWorkloadProfileDedicatedNodeEviction(t *testing.T) {
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

			// a process alive during the eviction keeps its node
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			alive := dockerInstance.CommandContext(ctx, workloadProfileExecPath("alive"), []string{"sleep", "60"}, []string{})
			if err := alive.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				cancel()
				_ = alive.Wait()
			}()

			et.trigger(t, dockerInstance, "known")
			if _, err := waitForWorkloadProfile(t, test, selector, func(p *profile.Profile) bool {
				return et.hasNode(p, "known") && profileHasProcess(p, "wp-exec-alive")
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
			assert.True(t, profileHasProcess(p, "wp-exec-alive"), "the node of a running process shouldn't be evicted")

			anomalies := collectAnomalies(t, test, func() error {
				et.trigger(t, dockerInstance, "known")
				return nil
			}, dockerInstance)
			assert.True(t, et.hasAnomalyFor(anomalies, "known"), "an evicted %s activity should trigger an anomaly again", et.name)
		})
	}
}
