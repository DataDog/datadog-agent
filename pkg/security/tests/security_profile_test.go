// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.SecurityProfile) bool {
				if sp.Version != "local_profile" {
					t.Errorf("Profile version %s != local_profile\n", sp.Version)
				}
				if sp.Metadata.Name != dump.Name {
					t.Errorf("Profile name %s != %s\n", sp.Metadata.Name, dump.Name)
				}
				if sp.Metadata.ContainerID != dump.ContainerID {
					t.Errorf("Profile containerID %s != %s\n", sp.Metadata.ContainerID, dump.ContainerID)
				}
				if !slices.Contains(sp.Tags, "container_id:"+dump.ContainerID) {
					t.Errorf("Profile did not contains container_id tag: %v\n", sp.Tags)
				}
				if !slices.Contains(sp.Tags, "image_tag:latest") {
					t.Errorf("Profile did not contains image_tag:latest %v\n", sp.Tags)
				}
				found := false
				for _, tag := range sp.Tags {
					if strings.HasPrefix(tag, "image_name:fake_ubuntu_") {
						found = true
						break
					}
				}
				if found == false {
					t.Errorf("Profile did not contains image_name tag: %v\n", sp.Tags)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.SecurityProfile) bool {
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil,
			func(sp *profile.SecurityProfile) bool {
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
					if name == "foo.bar" {
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.ExecEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		test.GetCustomEventSent(t, func() error {
			// don't do anything
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection.")
			}
			return false
		}, time.Second*3, model.ExecEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.com"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.DNSEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		test.GetCustomEventSent(t, func() error {
			// don't do anything
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection.")
			}
			return false
		}, time.Second*3, model.DNSEventType)
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
		anomalyDetectionMinimumStablePeriodExec: 0,
		anomalyDetectionMinimumStablePeriodDNS:  0,
		anomalyDetectionWarmupPeriod:            3 * time.Second,
		tagsResolver:                            NewFakeMonoResolver(),
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

	time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
	cmd := mainDockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
	cmd.CombinedOutput()
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	err = test.StopActivityDump(dump.Name, "", "")
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
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.bar"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*5, model.DNSEventType)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.bar"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*3, model.DNSEventType)
	})

	t.Run("anomaly-detection-warmed-up-not-autolearned-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.DNSEventType)
		if err != nil {
			t.Error(err)
		}
	})

	testDockerInstance2, _, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer testDockerInstance1.stop()

	t.Run("anomaly-detection-warmup-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*5, model.DNSEventType)
	})

	// already sleep for timeout for warmup period + 2sec spare (5s)

	t.Run("anomaly-detection-warmed-up-autolearned-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"foo.bar"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*3, model.DNSEventType)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-bis-2", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance2.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*3, model.DNSEventType)
	})

	t.Run("anomaly-detection-warmed-up-autolearned-bis-1", func(t *testing.T) {
		test.GetCustomEventSent(t, func() error {
			cmd := testDockerInstance1.Command("nslookup", []string{"foo.baz"}, []string{})
			cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			if r.Rule.ID == events.AnomalyDetectionRuleID {
				t.Fatal("Should not had receive any anomaly detection during warm up.")
			}
			return false
		}, time.Second*3, model.DNSEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*3, model.ExecEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*3, model.DNSEventType)
	})

	t.Run("anomaly-detection-stable-period-process", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second)  // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)
		time.Sleep(time.Second * 10) // waiting for the stable period

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.ExecEventType)
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

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(6 * time.Second)  // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)
		time.Sleep(time.Second * 10) // waiting for the stable period

		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"google.fr"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.DNSEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

}

func TestSecurityProfileAutoSuppression(t *testing.T) {
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

	var expectedFormats = []string{"profile", "protobuf"}
	var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

	outputDir := t.TempDir()
	os.MkdirAll(outputDir, 0755)
	defer os.RemoveAll(outputDir)
	reinsertPeriod := time.Second
	rulesDef := []*rules.RuleDefinition{
		{
			ID:         "test_autosuppression_exec",
			Expression: `exec.file.name == "getconf"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
		{
			ID:         "test_autosuppression_exec_2",
			Expression: `exec.file.name == "getent"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
		{
			ID:         "test_autosuppression_dns",
			Expression: `dns.question.type == A && dns.question.name == "foo.bar"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
		{
			ID:         "test_autosuppression_dns_2",
			Expression: `dns.question.type == A && dns.question.name == "foo.baz"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
	}
	test, err := newTestModule(t, nil, rulesDef, withStaticOpts(testOpts{
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
		anomalyDetectionMinimumStablePeriodExec: reinsertPeriod,
		anomalyDetectionMinimumStablePeriodDNS:  reinsertPeriod,
		anomalyDetectionWarmupPeriod:            reinsertPeriod,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
	cmd := dockerInstance.Command(syscallTester, []string{"sleep", "1"}, []string{})
	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

	t.Run("auto-suppression-process-signal", func(t *testing.T) {
		// check that we generate an event during profile learning phase
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			return assertTriggeredRule(t, rule, "test_autosuppression_exec") &&
				assert.Equal(t, "getconf", event.ProcessContext.FileEvent.BasenameStr, "wrong exec file")
		}, time.Second*3, "test_autosuppression_exec")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("auto-suppression-dns-signal", func(t *testing.T) {
		// check that we generate an event during profile learning phase
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			return assertTriggeredRule(t, rule, "test_autosuppression_dns") &&
				assert.Equal(t, "nslookup", event.ProcessContext.Argv0, "wrong exec file")
		}, time.Second*3, "test_autosuppression_dns")
		if err != nil {
			t.Fatal(err)
		}
	})

	err = test.StopActivityDump(dump.Name, "", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile to be loaded (5sec debounce + 1sec spare)

	t.Run("auto-suppression-process-suppression", func(t *testing.T) {
		// check we autosuppress signals
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == dump.ContainerID {
				t.Fatal("Got a signal that should have been suppressed")
			}
			return false
		}, time.Second*3, "test_autosuppression_exec")
		if err != nil {
			if otherErr, ok := err.(ErrTimeout); !ok {
				t.Fatal(otherErr)
			}
		}
	})

	t.Run("auto-suppression-dns-suppression", func(t *testing.T) {
		// check we autosuppress signals
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == dump.ContainerID {
				t.Fatal("Got a signal that should have been suppressed")
			}
			return false
		}, time.Second*3, "test_autosuppression_dns")
		if err != nil {
			if otherErr, ok := err.(ErrTimeout); !ok {
				t.Fatal(otherErr)
			}
		}
	})

	// let the profile became stable
	time.Sleep(reinsertPeriod)

	t.Run("auto-suppression-process-no-suppression", func(t *testing.T) {
		// check we don't autosuppress signals
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(rule *rules.Rule, event *model.Event) bool {
			return assertTriggeredRule(t, rule, "test_autosuppression_exec_2") &&
				assert.Equal(t, "getent", event.ProcessContext.FileEvent.BasenameStr, "wrong exec file")
		}, time.Second*3, "test_autosuppression_exec_2")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("auto-suppression-dns-no-suppression", func(t *testing.T) {
		// check we don't autosuppress signals
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"foo.baz"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(rule *rules.Rule, event *model.Event) bool {
			return assertTriggeredRule(t, rule, "test_autosuppression_dns_2") &&
				assert.Equal(t, "nslookup", event.ProcessContext.Argv0, "wrong exec file")
		}, time.Second*3, "test_autosuppression_dns_2")
		if err != nil {
			t.Fatal(err)
		}
	})
}
