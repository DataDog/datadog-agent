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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestSecurityProfile(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             200,
		activityDumpTracedCgroupsCount:      3,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		enableSecurityProfile:               true,
		securityProfileDir:                  outputDir,
		securityProfileWatchDir:             true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("security-profile-metadata", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				if sp.Metadata.Name != dump.Name {
					t.Errorf("Profile name %s != %s\n", sp.Metadata.Name, dump.Name)
				}
				if (sp.Metadata.ContainerID != dump.ContainerID) &&
					(sp.Metadata.CGroupContext.CGroupID != dump.CGroupID) {
					t.Errorf("Profile containerID %s != %s\n", sp.Metadata.ContainerID, dump.ContainerID)
				}

				ctx := sp.GetVersionContextIndex(0)
				if ctx == nil {
					t.Errorf("No profile context found!")
				} else {
					if !slices.Contains(ctx.Tags, "container_id:"+string(dump.ContainerID)) {
						t.Errorf("Profile did not contains container_id tag: %v\n", ctx.Tags)
					}
					if !slices.Contains(ctx.Tags, "image_tag:latest") {
						t.Errorf("Profile did not contains image_tag:latest %v\n", ctx.Tags)
					}
					found := false
					for _, tag := range ctx.Tags {
						if strings.HasPrefix(tag, "image_name:fake_ubuntu_") {
							found = true
							break
						}
					}
					if found == false {
						t.Errorf("Profile did not contains image_name tag: %v\n", ctx.Tags)
					}
				}
				return true
			})
	})

	t.Run("security-profile-process", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				nodes := WalkActivityTree(sp.ActivityTree, func(node *ProcessNodeAndParent) bool {
					return node.Node.Process.FileEvent.PathnameStr == syscallTester
				})
				if nodes == nil {
					t.Fatal("Node not found in security profile")
				}
				if len(nodes) != 1 {
					t.Fatalf("Found %d nodes, expected only one.", len(nodes))
				}
				return true
			})
	})

	t.Run("security-profile-dns", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO: Oracle because we are missing offsets. See dns_test.go
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
		})

		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				nodes := WalkActivityTree(sp.ActivityTree, func(node *ProcessNodeAndParent) bool {
					return node.Node.Process.Argv0 == "nslookup"
				})
				if nodes == nil {
					t.Fatal("Node not found in security profile")
				}
				if len(nodes) != 1 {
					t.Fatalf("Found %d nodes, expected only one.", len(nodes))
				}
				for name := range nodes[0].DNSNames {
					if name == "one.one.one.one" {
						return true
					}
				}
				t.Error("DNS req not found in security profile")
				return false
			})
	})
}

func TestAnomalyDetection(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
		anomalyDetectionMinimumStablePeriodExec: time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  time.Second,
		anomalyDetectionWarmupPeriod:            time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("anomaly-detection-process", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("anomaly-detection-process-negative", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		test.GetCustomEventSent(t, func() error {
			// don't do anything
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection.")
			return false
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-dns", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO: Oracle because we are missing offsets. See dns_test.go
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
		})
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.com"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("anomaly-detection-dns-negative", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO: Oracle because we are missing offsets. See dns_test.go
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
		})
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		test.GetCustomEventSent(t, func() error {
			// don't do anything
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection.")
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})
}

func TestAnomalyDetectionVariables(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	// Define a rule with a "set" action that matches exec events.
	// When both the rule and anomaly detection fire on the same event,
	// the variable set by the rule should be present in the anomaly
	// detection custom event.
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ad_variable_rule",
			Expression: `exec.file.name == "getconf"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Name:  "ad_test_var",
						Value: true,
					},
				},
			},
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
		anomalyDetectionMinimumStablePeriodExec: time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  time.Second,
		anomalyDetectionWarmupPeriod:            time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("anomaly-detection-variable-in-event", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		// Run the syscall_tester to populate the activity dump baseline.
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // let the profile load (5s debounce + 1s spare)

		// "getconf" is not in the baseline, so it triggers anomaly detection.
		// It also matches the rule "test_ad_variable_rule" which sets ad_test_var=true.
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, customEvent *events.CustomEvent) bool {
			// Verify the anomaly detection custom event carries the variable.
			eventJSON, jsonErr := customEvent.MarshalJSON()
			if jsonErr != nil {
				t.Errorf("failed to marshal custom event: %v", jsonErr)
				return false
			}

			jsonStr := string(eventJSON)

			// The event JSON should contain the variable set by the rule.
			if !assert.Contains(t, jsonStr, `"ad_test_var"`, "anomaly detection event should contain the variable name") {
				t.Logf("event JSON: %s", jsonStr)
				return false
			}

			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestAnomalyDetectionWarmup(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
		anomalyDetectionMinimumStablePeriodExec: 0,
		anomalyDetectionMinimumStablePeriodDNS:  0,
		anomalyDetectionWarmupPeriod:            3 * time.Second,
		tagger:                                  NewFakeMonoTagger(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal(err)
	}

	mainDockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer mainDockerInstance.stop()

	cmd := mainDockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
	cmd.CombinedOutput()
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	testDockerInstance1, _, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer testDockerInstance1.stop()

	t.Run("anomaly-detection-warmup-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"one.one.one.one"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*5, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"one.one.one.one"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-warmed-up-not-autolearned-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Error(err)
		}
	})

	testDockerInstance2, _, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer testDockerInstance2.stop()

	t.Run("anomaly-detection-warmup-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*5, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	// already sleep for timeout for warmup period + 2sec spare (5s)

	t.Run("anomaly-detection-warmed-up-autolearned-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"one.one.one.one"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-bis-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-bis-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not had receive any anomaly detection during warm up.")
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})
}

func TestSecurityProfileReinsertionPeriod(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            10 * time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("anomaly-detection-reinsertion-process", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-reinsertion-dns", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO: Oracle because we are missing offsets. See dns_test.go
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
		})
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("anomaly-detection-stable-period-process", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second)  // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)
		time.Sleep(time.Second * 10) // waiting for the stable period

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("anomaly-detection-stable-period-dns", func(t *testing.T) {
		checkKernelCompatibility(t, "RHEL, SLES and Oracle kernels", func(kv *kernel.Version) bool {
			// TODO: Oracle because we are missing offsets. See dns_test.go
			return kv.IsRH7Kernel() || kv.IsOracleUEKKernel() || kv.IsSLESKernel()
		})
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command("nslookup", []string{"one.one.one.one"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second)  // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)
		time.Sleep(time.Second * 10) // waiting for the stable period

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

}

func TestSecurityProfileDifferentiateArgs(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpCgroupDifferentiateArgs:     true,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec"},
		anomalyDetectionMinimumStablePeriodExec: time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  time.Second,
		anomalyDetectionWarmupPeriod:            time.Second,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
	cmd := dockerInstance.Command("/bin/date", []string{"-u"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	cmd = dockerInstance.Command("/bin/date", []string{"-R"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}

	// test profiling part
	validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil, func(sp *profile.Profile) bool {
		nodes := WalkActivityTree(sp.ActivityTree, func(node *ProcessNodeAndParent) bool {
			if node.Node.Process.FileEvent.PathnameStr == "/bin/date" || node.Node.Process.Argv0 == "/bin/date" {
				if len(node.Node.Process.Argv) == 1 && slices.Contains([]string{"-u", "-R"}, node.Node.Process.Argv[0]) {
					return true
				}
			}
			return false
		})
		if len(nodes) != 2 {
			t.Fatalf("found %d nodes, expected two.", len(nodes))
		}
		processNodesFound := uint32(0)
		for _, node := range nodes {
			if len(node.Process.Argv) == 1 && node.Process.Argv[0] == "-u" {
				processNodesFound |= 1
			} else if len(node.Process.Argv) == 1 && node.Process.Argv[0] == "-R" {
				processNodesFound |= 2
			}
		}
		if processNodesFound != (1 | 2) {
			t.Fatalf("could not find processes with expected arguments: %d", processNodesFound)
		}
		return true
	})

	// test matching part
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)
	err = test.GetCustomEventSent(t, func() error {
		cmd := dockerInstance.Command("/bin/date", []string{"--help"}, []string{})
		_, err = cmd.CombinedOutput()
		return err
	}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
		return true
	}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSecurityProfileLifeCycleExecs(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          10,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	t.Run("life-cycle-v1-learning-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	selector := fakeManualTagger.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "+",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is stable and V2 is learning

	t.Run("life-cycle-v2-learning-new-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("iconv", []string{"-l"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("life-cycle-v2-learning-v1-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("life-cycle-v1-stable-v2-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("iconv", []string{"-l"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-v1-unstable-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("scanelf", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})
}

func TestSecurityProfileLifeCycleDNS(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          10,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	t.Run("life-cycle-v1-learning-new-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	time.Sleep(time.Second * 10) // waiting for the stable period

	// HERE: V1 is stable

	t.Run("life-cycle-v1-stable-dns-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	selector := fakeManualTagger.GetContainerSelector(dockerInstanceV1.containerID)
	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "+",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is stable and V2 is learning

	t.Run("life-cycle-v2-learning-new-dns-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("nslookup", []string{"google.es"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	// most of the time DNS events triggers twice, let the second be handled before continuing
	time.Sleep(time.Second)

	t.Run("life-cycle-v2-learning-v1-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("life-cycle-v1-stable-v2-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.es"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-v1-unstable-new-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.co.uk"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
	})
}

func TestSecurityProfileLifeCycleSyscall(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "syscalls"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualResolver := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                         true,
		activityDumpRateLimiter:                    200,
		activityDumpTracedCgroupsCount:             10,
		activityDumpDuration:                       testActivityDumpDuration,
		activityDumpLocalStorageDirectory:          outputDir,
		activityDumpLocalStorageCompression:        false,
		activityDumpLocalStorageFormats:            expectedFormats,
		activityDumpTracedEventTypes:               testActivityDumpTracedEventTypes,
		enableSecurityProfile:                      true,
		securityProfileDir:                         outputDir,
		securityProfileWatchDir:                    true,
		enableAnomalyDetection:                     true,
		anomalyDetectionEventTypes:                 testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec:    10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:     10 * time.Second,
		anomalyDetectionDefaultMinimumStablePeriod: 10 * time.Second,
		anomalyDetectionWarmupPeriod:               1 * time.Second,
		tagger:                                     fakeManualResolver,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	// Some syscall will be missing from the initial dump because they had no way to come back to user space
	// (i.e. no new syscall to flush the dirty entry + no new exec + no new exit)
	t.Run("life-cycle-v1-learning", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("sleep", []string{"1"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, event *events.CustomEvent) bool {
			// We shouldn't see anything: the profile is still learning
			data, _ := event.MarshalJSON()
			t.Error(fmt.Errorf("syscall anomaly detected when it should have been ignored: %s", string(data)))
			// we answer false on purpose: we might have 2 or more syscall anomaly events
			return false
		}, time.Second*2, model.SyscallsEventType, events.AnomalyDetectionRuleID)
	})

	time.Sleep(time.Second * 10) // waiting for the stable period

	// HERE: V1 is stable

	t.Run("life-cycle-v1-stable-no-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("sleep", []string{"1"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, event *events.CustomEvent) bool {
			// this time we shouldn't see anything new.
			data, _ := event.MarshalJSON()
			t.Error(fmt.Errorf("syscall anomaly detected when it should have been ignored: %s", string(data)))
			return false
		}, time.Second*2, model.SyscallsEventType, events.AnomalyDetectionRuleID)
	})

	t.Run("life-cycle-v1-stable-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			// this will generate new syscalls, and should therefore generate an anomaly
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, _ *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.SyscallsEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	selector := fakeManualResolver.GetContainerSelector(dockerInstanceV1.containerID)
	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "+",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is stable and V2 is learning

	t.Run("life-cycle-v1-stable-v2-learning-anomaly", func(t *testing.T) {
		var gotSyscallsEvent bool
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("date", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, _ *events.CustomEvent) bool {
			// we should see an anomaly that will be inserted in the profile
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			gotSyscallsEvent = true
			// there may be multiple syscalls events
			return false
		}, time.Second*3, model.SyscallsEventType, events.AnomalyDetectionRuleID)
		if !gotSyscallsEvent {
			t.Fatal(err)
		}
	})

	t.Run("life-cycle-v1-stable-v2-learning-no-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("date", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, event *events.CustomEvent) bool {
			// this time we shouldn't see anything new.
			data, _ := event.MarshalJSON()
			t.Error(fmt.Errorf("syscall anomaly detected when it should have been ignored: %s", string(data)))
			return false
		}, time.Second*2, model.SyscallsEventType, events.AnomalyDetectionRuleID)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-v1-unstable-v2-learning", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, event *events.CustomEvent) bool {
			// We shouldn't see anything: the profile is unstable
			data, _ := event.MarshalJSON()
			t.Error(fmt.Errorf("syscall anomaly detected when it should have been ignored: %s", string(data)))
			// we answer false on purpose: we might have 2 or more syscall anomaly events
			return false
		}, time.Second*2, model.SyscallsEventType, events.AnomalyDetectionRuleID)
	})
}

func TestSecurityProfileLifeCycleEvictionProcess(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          10,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
		securityProfileMaxImageTags:             2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	t.Run("life-cycle-eviction-process-v1-learning-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	selector := fakeManualTagger.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-eviction-process-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v2",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is stable and V2 is learning

	t.Run("life-cycle-eviction-process-v2-learning-new-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("iconv", []string{"-l"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v3",
	})
	dockerInstanceV3, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV3.stop()

	// HERE: V1 is deleted, V2 is learning and V3 is learning

	t.Run("life-cycle-eviction-process-check-v1-evicted", func(t *testing.T) {
		versions, err := test.GetProfileVersions(selector.Image)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 2, len(versions))
		assert.True(t, slices.Contains(versions, selector.Tag+"v2"))
		assert.True(t, slices.Contains(versions, selector.Tag+"v3"))
		assert.False(t, slices.Contains(versions, selector.Tag))
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag+"v3", model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-process-v1-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("getconf", []string{"-a"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestSecurityProfileLifeCycleEvictionDNS(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          10,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
		securityProfileMaxImageTags:             2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	t.Run("life-cycle-eviction-dns-v1-learning-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
	})

	selector := fakeManualTagger.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-eviction-dns-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v2",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is stable and V2 is learning

	t.Run("life-cycle-eviction-dns-v2-learning-new-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("nslookup", []string{"google.es"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v3",
	})
	dockerInstanceV3, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV3.stop()

	// HERE: V1 is deleted, V2 is learning and V3 is learning

	t.Run("life-cycle-eviction-dns-check-v1-evicted", func(t *testing.T) {
		versions, err := test.GetProfileVersions(selector.Image)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 2, len(versions))
		assert.True(t, slices.Contains(versions, selector.Tag+"v2"))
		assert.True(t, slices.Contains(versions, selector.Tag+"v3"))
		assert.False(t, slices.Contains(versions, selector.Tag))
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag+"v3", model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-dns-v1-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("nslookup", []string{"google.fr"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.DNSEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestSecurityProfileLifeCycleEvictionProcessUnstable(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          10,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              testActivityDumpTracedEventTypes,
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
		securityProfileMaxImageTags:             2,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV1.stop()

	cmd := dockerInstanceV1.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	// HERE: V1 is learning

	t.Run("life-cycle-eviction-process-unstable-v1-learning-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	selector := fakeManualTagger.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, model.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable

	t.Run("life-cycle-eviction-process-unstable-v1-unstable", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v2",
	})
	dockerInstanceV2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV2.stop()

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-eviction-process-unstable-v2-learning", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("iconv", []string{"-l"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	fakeManualTagger.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   selector.Tag + "v3",
	})
	dockerInstanceV3, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstanceV3.stop()

	// HERE: V1 is deleted, V2 is learning and V3 is learning

	t.Run("life-cycle-eviction-process-unstable-v3-learning", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("getconf", []string{"-a"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag+"v3", model.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-process-unstable-v3-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestSecurityProfilePersistence(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	fakeManualTagger := NewFakeManualTagger()

	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          3,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec"},
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagger:                                  fakeManualTagger,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dockerInstance1, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance1.stop()

	err = test.StopActivityDump(dump.Name)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

	// add anomaly test event during reinsertion period
	_, err = dockerInstance1.Command("/bin/echo", []string{"aaa"}, []string{}).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Second) // wait for the stable period
	_, err = dockerInstance1.Command("/bin/echo", []string{"aaa"}, []string{}).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // quick sleep to let the exec event state become stable

	// stop the container so that the profile gets persisted
	dockerInstance1.stop()

	// make sure the next instance has the same image name as the previous one
	fakeManualTagger.SpecifyNextSelector(fakeManualTagger.GetContainerSelector(dockerInstance1.containerID))
	dockerInstance2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance2.stop()
	time.Sleep(10 * time.Second) // sleep to let the profile be loaded (directory provider debouncers)

	// check the profile is still applied, and anomaly events can be generated
	t.Run("persistence-anomaly-check", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			dockerInstance2.Command("getent", []string{}, []string{}).CombinedOutput()
			return nil
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	// check the profile is still applied, and anomalies aren't generated for known events
	t.Run("persistence-no-anomaly-check", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			_, err := dockerInstance2.Command("/bin/echo", []string{"aaa"}, []string{}).CombinedOutput()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return false
		}, time.Second*2, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			if otherErr, ok := err.(ErrTimeout); !ok {
				t.Fatal(otherErr)
			}
		}
	})
}

// TestSecurityProfileSystemd tests the security profile functionality for systemd services.
// It verifies that security profiles are correctly generated for systemd-managed services,
// including proper metadata extraction and process tree capture.
func TestSecurityProfileSystemd(t *testing.T) {
	SkipIfNotAvailable(t)

	// Skip if not running on a systemd system
	if !isSystemdAvailable() {
		t.Skip("Skip test when systemd is not available")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             200,
		activityDumpTracedCgroupsCount:      100,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		enableSecurityProfile:               true,
		securityProfileDir:                  outputDir,
		securityProfileWatchDir:             true,
		traceSystemdCgroups:                 true,
		enableSBOM:                          true,
		enableHostSBOM:                      true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// Test that our systemd service profile metadata is correctly generated
	// This test verifies that profile metadata includes service name tags and correct cgroup information
	t.Run("systemd-service-profile-metadata", func(t *testing.T) {
		serviceName := "cws-test-service-" + utils.RandString(6)
		reloadCmd := syscallTester + " sleep 1"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		defer serviceInstance.stop()
		if err != nil {
			t.Fatal(err)
		}

		// reload the service to execute the reload command
		if err := serviceInstance.reload(); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				if sp.Metadata.Name != dump.Name {
					t.Errorf("Profile name %s != %s\n", sp.Metadata.Name, dump.Name)
				}
				if sp.Metadata.CGroupContext.CGroupID != dump.CGroupID {
					t.Errorf("Profile cgroup ID %s != %s\n", sp.Metadata.CGroupContext.CGroupID, dump.CGroupID)
				}

				ctx := sp.GetVersionContextIndex(0)
				if ctx == nil {
					t.Errorf("No profile context found!")
				} else {
					if !slices.Contains(ctx.Tags, "service:"+serviceName+".service") {
						t.Errorf("Profile did not contain service tag: %v\n", ctx.Tags)
					}
				}
				return true
			})
	})

	// Test that systemd service process information is correctly captured in profiles
	// This test verifies that the process tree includes the expected executables run within the service
	t.Run("systemd-service-profile-process", func(t *testing.T) {
		serviceName := "cws-test-service-proc-" + utils.RandString(6)
		reloadCmd := syscallTester + " sleep 1"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		defer serviceInstance.stop()
		if err != nil {
			t.Fatal(err)
		}

		// reload the service to execute the reload command
		if err := serviceInstance.reload(); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				nodes := WalkActivityTree(sp.ActivityTree, func(node *ProcessNodeAndParent) bool {
					return node.Node.Process.FileEvent.PathnameStr == syscallTester
				})

				if nodes == nil {
					t.Fatal("Node not found in systemd service security profile")
				}
				if len(nodes) != 1 {
					t.Fatalf("Found %d nodes, expected only one.", len(nodes))
				}
				return true
			})
	})
}

func TestAnomalyDetectionSystemd(t *testing.T) {
	SkipIfNotAvailable(t)

	// Skip if not running on a systemd system
	if !isSystemdAvailable() {
		t.Skip("Skip test when systemd is not available")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          100,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
		anomalyDetectionMinimumStablePeriodExec: time.Second,
		anomalyDetectionMinimumStablePeriodDNS:  time.Second,
		anomalyDetectionWarmupPeriod:            time.Second,
		traceSystemdCgroups:                     true,
		enableSBOM:                              true,
		enableHostSBOM:                          true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// Test that anomaly detection correctly identifies unknown processes in systemd services
	// This test verifies that executing a process not in the security profile triggers an anomaly detection event
	t.Run("systemd-anomaly-detection-process", func(t *testing.T) {
		serviceName := "cws-test-service-anomaly-pos-" + utils.RandString(6)
		reloadCmd := "getconf -a"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		if err != nil {
			t.Fatal(err)
		}
		defer serviceInstance.stop()

		// stop the activity dump before reloading the service so that the reload command can be considered as an anomaly
		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			// Execute the reload command to trigger an anomaly detection event
			err := serviceInstance.reload()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	// Test that anomaly detection doesn't trigger false positives for known processes
	// This test verifies that executing a process that exists in the security profile does not trigger an anomaly
	t.Run("systemd-anomaly-detection-process-negative", func(t *testing.T) {
		serviceName := "cws-test-service-anomaly-neg-" + utils.RandString(6)
		reloadCmd := syscallTester + " sleep 1"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		if err != nil {
			t.Fatal(err)
		}
		defer serviceInstance.stop()

		// reload the service to execute the reload command so that the command is considered as part of the profile
		err = serviceInstance.reload()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

		test.GetCustomEventSent(t, func() error {
			// Execute the same command that was profiled - should not trigger anomaly
			err := serviceInstance.reload()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not have received any anomaly detection for known command.")
			return false
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	})
}

// TestSecurityProfileSystemdLifeCycle tests the lifecycle management of security profiles for systemd services.
// It verifies that profiles transition correctly between learning and stable states, and that
// multiple versions of the same service are handled properly with appropriate anomaly detection behavior.
func TestSecurityProfileSystemdLifeCycle(t *testing.T) {
	SkipIfNotAvailable(t)

	// Skip if not running on a systemd system
	if !isSystemdAvailable() {
		t.Skip("Skip test when systemd is not available")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                      true,
		activityDumpRateLimiter:                 200,
		activityDumpTracedCgroupsCount:          100,
		activityDumpDuration:                    testActivityDumpDuration,
		activityDumpLocalStorageDirectory:       outputDir,
		activityDumpLocalStorageCompression:     false,
		activityDumpLocalStorageFormats:         expectedFormats,
		activityDumpTracedEventTypes:            testActivityDumpTracedEventTypes,
		enableSecurityProfile:                   true,
		securityProfileDir:                      outputDir,
		securityProfileWatchDir:                 true,
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec"},
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		traceSystemdCgroups:                     true,
		enableSBOM:                              true,
		enableHostSBOM:                          true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// Test that new processes are automatically learned during the learning phase
	// This test verifies that processes executed during the learning phase are added to the profile
	// and don't trigger anomaly detection events
	t.Run("systemd-lifecycle-v1-learning-new-process", func(t *testing.T) {
		serviceName := "cws-test-lifecycle-learning-" + utils.RandString(6)
		reloadCmd := syscallTester + " sleep 1"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		if err != nil {
			t.Fatal(err)
		}
		defer serviceInstance.stop()

		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

		// HERE: V1 is learning - new process should not trigger anomaly
		test.GetCustomEventSent(t, func() error {
			err := serviceInstance.reload()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not have received any anomaly detection during learning phase.")
			return false
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	})

	// Test that unknown processes trigger anomalies when the profile is stable
	// This test verifies that once a profile transitions to stable state,
	// executing processes not in the profile generates anomaly detection events
	t.Run("systemd-lifecycle-v1-stable-process-anomaly", func(t *testing.T) {
		serviceName := "cws-test-lifecycle-stable-" + utils.RandString(6)
		reloadCmd := syscallTester + " sleep 1"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		if err != nil {
			t.Fatal(err)
		}
		defer serviceInstance.stop()

		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

		// Wait for the stable period to pass
		time.Sleep(11 * time.Second)

		err = test.GetCustomEventSent(t, func() error {
			// Execute the new reload command, it should trigger an anomaly
			err := serviceInstance.reload()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			return true
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
		if err != nil {
			t.Fatal(err)
		}
	})

	// Test that known processes don't trigger anomalies when the profile is stable
	// This test verifies that processes that exist in the security profile do not trigger anomalies
	t.Run("systemd-lifecycle-v1-stable-known-process", func(t *testing.T) {
		serviceName := "cws-test-lifecycle-known-" + utils.RandString(6)
		reloadCmd := "getconf -a"
		serviceInstance, dump, err := test.StartSystemdServiceGetDump(serviceName, reloadCmd)
		if err != nil {
			t.Fatal(err)
		}
		defer serviceInstance.stop()

		// reload the service to execute the reload command so it gets profiled
		if err := serviceInstance.reload(); err != nil {
			t.Fatal(err)
		}

		time.Sleep(3 * time.Second) // a quick sleep to let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

		// Wait for the stable period to pass
		time.Sleep(11 * time.Second)

		// HERE: V1 is stable - known process should not trigger anomaly
		test.GetCustomEventSent(t, func() error {
			// Execute the same command that was profiled - should not trigger anomaly
			err := serviceInstance.reload()
			return err
		}, func(_ *rules.Rule, _ *events.CustomEvent) bool {
			t.Error("Should not have received any anomaly detection for known command.")
			return false
		}, time.Second*3, model.ExecEventType, events.AnomalyDetectionRuleID)
	})
}

func TestSecurityProfileNodeEviction(t *testing.T) {
	SkipIfNotAvailable(t)

	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNodeForAD() {
		t.Skip("Skip test when not run in dedicated env")
	}

	var expectedFormats = []string{"profile"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             200,
		activityDumpTracedCgroupsCount:      3,
		activityDumpDuration:                3 * time.Minute,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		anomalyDetectionEventTypes:          []string{"exec", "syscalls", "dns", "open"},
		enableSecurityProfile:               true,
		enableAnomalyDetection:              true,
		securityProfileDir:                  outputDir,
		securityProfileWatchDir:             true,
		securityProfileNodeEvictionTimeout:  5 * time.Second,
		anomalyDetectionWarmupPeriod:        2 * time.Minute, // as we don't have the new lifecyle of the profiles in which we reinject the drift nodes, we need to be in warmup period to make sure that the new activities of child2 are reinjected
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("node-eviction-basic", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		activities := [][]string{
			{syscallTester, "sleep", "1"},
			{"touch", "/tmp/test_file"},
			{"nslookup", "example.com"},
		}

		for _, activity := range activities {
			cmd := dockerInstance.Command(activity[0], activity[1:], []string{})
			_, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatal(err)
			}
		}

		time.Sleep(1 * time.Second) // Let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		var imageName string
		// Verify profile was created with nodes
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				imageName, _ = sp.GetImageNameTag()
				// Check that we have the activities are in the profile
				nodes := WalkActivityTree(sp.ActivityTree, func(node *ProcessNodeAndParent) bool {
					for _, activity := range activities {
						if node.Node.Process.Argv0+" "+strings.Join(node.Node.Process.Argv, " ") == strings.Join(activity, " ") {
							return true
						}
					}
					return false
				})

				if len(nodes) != len(activities) {
					t.Errorf("Expected %d process nodes found in profile, got %d", len(activities), len(nodes))
					return false
				}

				return true
			})

		// Wait for eviction timeout + some buffer
		// we need to wait at least twice the eviction timeout
		// because at the worst case, a node can be touched right after an eviction tick
		time.Sleep(10 * time.Second)

		manager := test.probe.PlatformProbe.(*probe.EBPFProbe).GetProfileManager()
		if err != nil {
			t.Fatal(err)
		}
		profile := manager.(*securityprofile.Manager).GetProfile(cgroupModel.WorkloadSelector{Image: imageName, Tag: "*"})
		if profile == nil {
			t.Fatal("profile is nil")
		}

		profile.Lock()
		defer profile.Unlock()

		// Verify that the nodes have been evicted
		nodes := WalkActivityTree(profile.ActivityTree, func(node *ProcessNodeAndParent) bool {
			for _, activity := range activities {
				if node.Node.Process.Argv0+" "+strings.Join(node.Node.Process.Argv, " ") == strings.Join(activity, " ") {
					return true
				}
			}
			return false
		})

		if len(nodes) > 0 {
			t.Errorf("Process nodes found in profile: %d", len(nodes))
		}

	})

	t.Run("node-eviction-partial-children", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		// Create parent process that spawns two child processes
		// Use a simple approach: parent shell spawns two background children and waits
		// child 1 does one operation and exits
		// child 2 does keep doing operations
		cmd := dockerInstance.Command("sh", []string{"-c", `
		    echo "parent process started" >&2
		    # Spawn child 1 in background - does one operation and exits
		    touch /tmp/child1_file &
		    child1_pid=$!
		    # Spawn child 2 in background - does operation, sleeps, then does it again
		    (for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do ls /tmp; sleep 1; done) &
		    child2_pid=$!
		    wait $child1_pid
		    wait $child2_pid
		    echo "parent process ended" >&2
		`}, []string{})

		err = cmd.Start()
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(1 * time.Second) // Let events be added to the dump

		err = test.StopActivityDump(dump.Name)
		if err != nil {
			t.Fatal(err)
		}

		var imageName string
		// Verify profile was created with nodes
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.Profile) bool {
				imageName, _ = sp.GetImageNameTag()
				nodes := WalkActivityTree(sp.ActivityTree, func(_ *ProcessNodeAndParent) bool {
					return true
				})

				// We shoud have 4 nodes: the base sleep activity, the parent, the child 1 and the child 2
				if len(nodes) != 4 {
					t.Errorf("Expected 4 nodes, got %d", len(nodes))
					return false
				}

				return true
			})

		// Child 2 will ls again after 7 seconds, so it should be kept
		// Wait for child 1 to be evicted
		time.Sleep(11 * time.Second)

		manager := test.probe.PlatformProbe.(*probe.EBPFProbe).GetProfileManager()
		profile := manager.(*securityprofile.Manager).GetProfile(cgroupModel.WorkloadSelector{Image: imageName, Tag: "*"})
		if profile == nil {
			t.Fatal("profile is nil")
		}

		profile.Lock()
		defer profile.Unlock()

		// Count remaining nodes
		allNodes := WalkActivityTree(profile.ActivityTree, func(_ *ProcessNodeAndParent) bool {
			return true
		})

		// we should have 2 nodes left:  parent and child 2
		if len(allNodes) != 2 {
			t.Errorf("Expected 2 nodes left, got %d", len(allNodes))
		}

		var argv0s []string
		for _, node := range allNodes {
			argv0s = append(argv0s, node.Process.Argv0)
		}

		// check that parent is not evicted
		if !slices.Contains(argv0s, "sh") {
			t.Errorf("Parent should not have been evicted, got %v", argv0s)
		}

		// check that child 2 is not evicted
		if !slices.Contains(argv0s, "ls") {
			t.Errorf("Child 2 should not have been evicted, got %v", argv0s)
		}

		// check that child 1 is evicted
		if slices.Contains(argv0s, "touch") {
			t.Errorf("Child 1 should have been evicted, got %v", argv0s)
		}

		// Wait for the background process to complete
		_ = cmd.Wait()
		t.Cleanup(func() {
			if cmd.Process != nil {
				// stop the sleep process
				cmd.Process.Kill()
			}
		})

	})

}
