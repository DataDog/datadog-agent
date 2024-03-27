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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
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
				if sp.Metadata.Name != dump.Name {
					t.Errorf("Profile name %s != %s\n", sp.Metadata.Name, dump.Name)
				}
				if sp.Metadata.ContainerID != dump.ContainerID {
					t.Errorf("Profile containerID %s != %s\n", sp.Metadata.ContainerID, dump.ContainerID)
				}

				ctx := sp.GetVersionContextIndex(0)
				if ctx == nil {
					t.Errorf("No profile context found!")
				} else {
					if !slices.Contains(ctx.Tags, "container_id:"+dump.ContainerID) {
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
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec", "dns"},
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
	defer testDockerInstance2.stop()

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
		enableAutoSuppression:                   true,
		autoSuppressionEventTypes:               []string{"exec", "dns"},
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

	err = test.StopActivityDump(dump.Name, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// test profiling part
	validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, nil, func(sp *profile.SecurityProfile) bool {
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
	}, func(r *rules.Rule, event *events.CustomEvent) bool {
		assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
		return true
	}, time.Second*3, model.ExecEventType)
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

	fakeManualResolver := NewFakeManualResolver()

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
		tagsResolver:                            fakeManualResolver,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, err := test.StartADocker()
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

	err = test.StopActivityDump("", dockerInstanceV1.containerID, "")
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	selector := fakeManualResolver.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

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

	t.Run("life-cycle-v2-learning-new-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("iconv", []string{"-l"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("life-cycle-v2-learning-v1-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	t.Run("life-cycle-v1-stable-v2-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("iconv", []string{"-l"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-v1-unstable-new-process", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("scanelf", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType)
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

	fakeManualResolver := NewFakeManualResolver()

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
		tagsResolver:                            fakeManualResolver,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	dockerInstanceV1, err := test.StartADocker()
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

	err = test.StopActivityDump("", dockerInstanceV1.containerID, "")
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType)
	})

	time.Sleep(time.Second * 10) // waiting for the stable period

	// HERE: V1 is stable

	t.Run("life-cycle-v1-stable-dns-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.DNSEventType)
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

	t.Run("life-cycle-v2-learning-new-dns-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV2.Command("nslookup", []string{"google.es"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*3, model.DNSEventType)
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType)
	})

	t.Run("life-cycle-v1-stable-v2-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.es"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable and V2 is learning

	t.Run("life-cycle-v1-unstable-new-dns", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.co.uk"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.DNSEventType)
	})
}

func TestSecurityProfileLifeCycleEvictitonProcess(t *testing.T) {
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

	fakeManualResolver := NewFakeManualResolver()

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
		tagsResolver:                            fakeManualResolver,
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

	dockerInstanceV1, err := test.StartADocker()
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

	err = test.StopActivityDump("", dockerInstanceV1.containerID, "")
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	selector := fakeManualResolver.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-eviction-process-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
	}, selector.Tag+"v3", profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-process-v1-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("getconf", []string{"-a"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestSecurityProfileLifeCycleEvictitonDNS(t *testing.T) {
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

	fakeManualResolver := NewFakeManualResolver()

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
		tagsResolver:                            fakeManualResolver,
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

	dockerInstanceV1, err := test.StartADocker()
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

	err = test.StopActivityDump("", dockerInstanceV1.containerID, "")
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.DNSEventType)
	})

	selector := fakeManualResolver.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is stable

	t.Run("life-cycle-eviction-dns-v1-stable-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("nslookup", []string{"google.com"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.DNSEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.DNSEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
	}, selector.Tag+"v3", profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-dns-v1-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("nslookup", []string{"google.fr"}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.DNSEventType)
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestSecurityProfileLifeCycleEvictitonProcessUnstable(t *testing.T) {
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

	fakeManualResolver := NewFakeManualResolver()

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
		tagsResolver:                            fakeManualResolver,
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

	dockerInstanceV1, err := test.StartADocker()
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

	err = test.StopActivityDump("", dockerInstanceV1.containerID, "")
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been reinserted"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	selector := fakeManualResolver.GetContainerSelector(dockerInstanceV1.containerID)
	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag, profile.UnstableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is unstable

	t.Run("life-cycle-eviction-process-unstable-v1-unstable", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV1.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	fakeManualResolver.SpecifyNextSelector(&cgroupModel.WorkloadSelector{
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
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			t.Fatal(errors.New("catch a custom event that should had been discarded"))
			return false
		}, time.Second*2, model.ExecEventType)
	})

	if err := test.SetProfileVersionState(&cgroupModel.WorkloadSelector{
		Image: selector.Image,
		Tag:   "*",
	}, selector.Tag+"v3", profile.StableEventType); err != nil {
		t.Fatal(err)
	}

	// HERE: V1 is deleted, V2 is learning and V3 is stable

	t.Run("life-cycle-eviction-process-unstable-v3-process-anomaly", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			cmd := dockerInstanceV3.Command("getent", []string{}, []string{})
			_, _ = cmd.CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return true
		}, time.Second*2, model.ExecEventType)
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

	rulesDef := []*rules.RuleDefinition{
		{
			ID:         "test_autosuppression_exec",
			Expression: `exec.file.name == "getconf"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
	}

	fakeManualResolver := NewFakeManualResolver()

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
		enableAutoSuppression:                   true,
		autoSuppressionEventTypes:               []string{"exec"},
		enableAnomalyDetection:                  true,
		anomalyDetectionEventTypes:              []string{"exec"},
		anomalyDetectionMinimumStablePeriodExec: 10 * time.Second,
		anomalyDetectionWarmupPeriod:            1 * time.Second,
		tagsResolver:                            fakeManualResolver,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dockerInstance1, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance1.stop()

	err = test.StopActivityDump("", dockerInstance1.containerID, "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second) // a quick sleep to let the profile be loaded (5sec debounce + 1sec spare)

	// add auto-suppression test event during reinsertion period
	_, err = dockerInstance1.Command("getconf", []string{"-a"}, []string{}).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

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
	fakeManualResolver.SpecifyNextSelector(fakeManualResolver.GetContainerSelector(dockerInstance1.containerID))
	dockerInstance2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance2.stop()
	time.Sleep(10 * time.Second) // sleep to let the profile be loaded (directory provider debouncers)

	// check the profile is still applied, and events can be auto suppressed
	t.Run("persistence-autosuppression-check", func(t *testing.T) {
		err = test.GetEventSent(t, func() error {
			_, err := dockerInstance2.Command("getconf", []string{"-a"}, []string{}).CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			t.Fatal("Got an event that should have been suppressed")
			return false
		}, time.Second*3, "test_autosuppression_exec")
		if err != nil {
			if otherErr, ok := err.(ErrTimeout); !ok {
				t.Fatal(otherErr)
			}
		}
	})

	// check the profile is still applied, and anomaly events can be generated
	t.Run("persistence-anomaly-check", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			dockerInstance2.Command("getent", []string{}, []string{}).CombinedOutput()
			return nil
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			return assert.Equal(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
		}, time.Second*2, model.ExecEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	// check the profile is still applied, and anomalies aren't generated for known events
	t.Run("persistence-no-anomaly-check", func(t *testing.T) {
		err = test.GetCustomEventSent(t, func() error {
			_, err := dockerInstance2.Command("/bin/echo", []string{"aaa"}, []string{}).CombinedOutput()
			return err
		}, func(r *rules.Rule, event *events.CustomEvent) bool {
			assert.NotEqual(t, events.AnomalyDetectionRuleID, r.Rule.ID, "wrong custom event rule ID")
			return false
		}, time.Second*2, model.ExecEventType)
		if err != nil {
			if otherErr, ok := err.(ErrTimeout); !ok {
				t.Fatal(otherErr)
			}
		}
	})
}
