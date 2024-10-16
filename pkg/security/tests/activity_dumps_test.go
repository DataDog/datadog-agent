// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	activitydump "github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"

	"github.com/stretchr/testify/assert"
)

var testActivityDumpCleanupPeriod = 15 * time.Second

func TestActivityDumps(t *testing.T) {
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

	outputDir := t.TempDir()

	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind", "imds"}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpCleanupPeriod:           testActivityDumpCleanupPeriod,
		networkIngressEnabled:               true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("activity-dump-cgroup-imds", func(t *testing.T) {
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
		cmd := dockerInstance.Command(goSyscallTester, []string{"-setup-and-run-imds-test"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes(goSyscallTester)
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}

			var requestFound, responseFound bool
			for _, node := range nodes {
				for evt := range node.IMDSEvents {
					if evt.Type == "request" && evt.URL == testutils.IMDSSecurityCredentialsURL {
						requestFound = true
					}
					if evt.Type == "response" && evt.AWS.SecurityCredentials.AccessKeyID == testutils.AWSSecurityCredentialsAccessKeyIDTestValue {
						responseFound = true
					}
				}
			}
			return requestFound && responseFound
		}, nil)
	})

	t.Run("activity-dump-cgroup-process", func(t *testing.T) {
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

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes(syscallTester)
			if nodes == nil {
				t.Fatalf("Node not found in activity dump: %+v", nodes)
			}
			if len(nodes) != 1 {
				t.Fatalf("Found %d nodes, expected only one.", len(nodes))
			}
			node := nodes[0]

			// ProcessActivityNode content1
			assert.Equal(t, activity_tree.Runtime, node.GenerationType)
			assert.Equal(t, 0, len(node.Children))
			assert.Equal(t, 0, len(node.Files))
			assert.Equal(t, 0, len(node.DNSNames))
			assert.Equal(t, 0, len(node.Sockets))

			// Process content
			assert.Equal(t, node.Process.Pid, node.Process.Tid)
			assert.Equal(t, uint32(0), node.Process.UID)
			assert.Equal(t, uint32(0), node.Process.GID)
			assert.Equal(t, "root", node.Process.User)
			assert.Equal(t, "root", node.Process.Group)
			if node.Process.Pid < node.Process.PPid {
				t.Errorf("PID < PPID")
			}
			assert.Equal(t, "", node.Process.TTYName)
			assert.Equal(t, "syscall_tester", node.Process.Comm)
			assert.Equal(t, false, node.Process.ArgsTruncated)
			assert.Equal(t, 2, len(node.Process.Argv))
			if len(node.Process.Argv) >= 2 {
				assert.Equal(t, "sleep", node.Process.Argv[0])
				assert.Equal(t, "1", node.Process.Argv[1])
			}
			assert.Equal(t, syscallTester, node.Process.Argv0)
			assert.Equal(t, false, node.Process.ArgsTruncated)
			assert.Equal(t, false, node.Process.EnvsTruncated)
			found := false
			for _, env := range node.Process.Envs {
				if strings.HasPrefix(env, "PATH=") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("PATH not present in envs")
			}

			// Process.FileEvent content
			if !strings.HasSuffix(node.Process.FileEvent.PathnameStr, "/syscall_tester") {
				t.Errorf("PathnameStr did not ends with /syscall_tester: %s", node.Process.FileEvent.PathnameStr)
			}
			assert.Equal(t, "syscall_tester", node.Process.FileEvent.BasenameStr)
			return true
		}, nil)
	})

	t.Run("activity-dump-cgroup-bind", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes(syscallTester)
			if nodes == nil {
				t.Fatalf("Node not found in activity dump: %+v", nodes)
			}
			for _, node := range nodes {
				for _, s := range node.Sockets {
					if s.Family == "AF_INET" {
						for _, bindNode := range s.Bind {
							if bindNode.Port == 4242 && bindNode.IP == "0.0.0.0" {
								return true
							}
						}
					}
				}
			}
			return false
		}, nil)
	})

	t.Run("activity-dump-cgroup-dns", func(t *testing.T) {
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

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes("nslookup")
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			for _, node := range nodes {
				for name := range node.DNSNames {
					if name == "foo.bar" {
						return true
					}
				}
			}
			return false
		}, nil)
	})

	t.Run("activity-dump-cgroup-file", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		temp, _ := os.CreateTemp(test.st.Root(), "ad-test-create")
		os.Remove(temp.Name()) // next touch command have to create the file

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command("touch", []string{temp.Name()}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		tempPathParts := strings.Split(temp.Name(), "/")
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes("touch")
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			for _, node := range nodes {
				current := node.Files
				for _, part := range tempPathParts {
					if part == "" {
						continue
					}
					next, found := current[part]
					if !found {
						return false
					}
					current = next.Children
				}
			}
			return true
		}, nil)
	})

	t.Run("activity-dump-cgroup-syscalls", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
		cmd := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes(syscallTester)
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			var exitOK, bindOK bool
			for _, node := range nodes {
				for _, s := range node.Syscalls {
					if s == int(model.SysExit) || s == int(model.SysExitGroup) {
						exitOK = true
					}
					if s == int(model.SysBind) {
						bindOK = true
					}
				}
			}
			if !exitOK {
				t.Errorf("exit syscall not found in activity dump")
			}
			if !bindOK {
				t.Errorf("bind syscall not found in activity dump")
			}
			return exitOK && bindOK
		}, nil)
	})

	t.Run("activity-dump-cgroup-rate-limiter", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		testDir := filepath.Join(test.st.Root(), "ratelimiter")
		if err := os.MkdirAll(testDir, os.ModePerm); err != nil {
			t.Fatal(err)
		}
		var files []string
		for i := 0; i < testActivityDumpRateLimiter*10; i++ {
			files = append(files, filepath.Join(testDir, "ad-test-create-"+fmt.Sprintf("%d", i)))
		}
		args := []string{"sleep", "2", ";", "open"}
		args = append(args, files...)
		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited before starting
		cmd := dockerInstance.Command(syscallTester, args, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingRootNodes(syscallTester)
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			if len(nodes) != 1 {
				t.Fatal("Captured more than one testsuite node")
			}

			tempPathParts := strings.Split(testDir, "/")
			for _, node := range nodes {
				current := node.Files
				for _, part := range tempPathParts {
					if part == "" {
						continue
					}
					next, found := current[part]
					if !found {
						return false
					}
					current = next.Children
					numberOfFiles := len(current)
					if part == "ratelimiter" && (numberOfFiles < testActivityDumpRateLimiter/4 || numberOfFiles > testActivityDumpRateLimiter) {
						t.Fatalf("Didn't find the good number of files in tmp node (%d/%d)", numberOfFiles, testActivityDumpRateLimiter)
					}
				}
			}
			return true
		}, nil)
	})

	t.Run("activity-dump-cgroup-timeout", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		// check that the dump is still alive
		time.Sleep(testActivityDumpDuration - 10*time.Second)
		assert.Equal(t, true, test.isDumpRunning(dump))

		// check that the dump has timeouted after the cleanup period + 10s + 2s
		time.Sleep(testActivityDumpCleanupPeriod + 12*time.Second)
		assert.Equal(t, false, test.isDumpRunning(dump))
	})

	t.Run("activity-dump-cgroup-counts", func(t *testing.T) {
		// first, stop all running activity dumps
		err := test.StopAllActivityDumps()
		if err != nil {
			t.Fatal("Can't stop all running activity dumps")
		}

		// then, launch enough docker instances to reach the testActivityDumpCgroupDumpTimeout
		var startedDumps []*activityDumpIdentifier
		for i := 0; i < testActivityDumpTracedCgroupsCount; i++ {
			dockerInstance, dump, err := test.StartADockerGetDump()
			if err != nil {
				t.Fatal(err)
			}
			defer dockerInstance.stop()
			startedDumps = append(startedDumps, dump)
		}

		// verify that we have the corresponding running dumps
		dumps, err := test.ListActivityDumps()
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, testActivityDumpTracedCgroupsCount, len(dumps))
		if !isListOfDumpsEqual(startedDumps, dumps) {
			t.Fatal("List of active dumps don't match the started ones")
		}

		// then, start an extra one and check that it's not dumping
		dockerInstance, err := test.StartADocker()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()
		dumps, err = test.ListActivityDumps()
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, testActivityDumpTracedCgroupsCount, len(dumps))
	})
}

func isListOfDumpsEqual(list1, list2 []*activityDumpIdentifier) bool {
	if len(list1) != len(list2) {
		return false
	}
firstLoop:
	for _, l1 := range list1 {
		for _, l2 := range list2 {
			if l1.Name == l2.Name {
				continue firstLoop
			}
		}
		return false
	}
	return true
}

func TestActivityDumpsAutoSuppression(t *testing.T) {
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

	rulesDef := []*rules.RuleDefinition{
		{
			ID:         "test_autosuppression_exec",
			Expression: `exec.file.name == "getconf"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
		{
			ID:         "test_autosuppression_dns",
			Expression: `dns.question.type == A && dns.question.name == "foo.bar"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
	}

	test, err := newTestModule(t, nil, rulesDef, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpAutoSuppressionEnabled:  true,
		autoSuppressionEventTypes:           []string{"exec", "dns"},
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
	t.Run("auto-suppression-process-suppression", func(t *testing.T) {
		// check we autosuppress signals during the activity dump duration
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("getconf", []string{"-a"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == containerutils.ContainerID(dump.ContainerID) {
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
		// check we autosuppress signals during the activity dump duration
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance.Command("nslookup", []string{"foo.bar"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == containerutils.ContainerID(dump.ContainerID) {
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
}

func TestActivityDumpsAutoSuppressionDriftOnly(t *testing.T) {
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

	rulesDef := []*rules.RuleDefinition{
		{
			ID:         "test_autosuppression_exec",
			Expression: `exec.file.name == "getconf"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
		{
			ID:         "test_autosuppression_dns",
			Expression: `dns.question.type == A && dns.question.name == "foo.bar"`,
			Tags:       map[string]string{"allow_autosuppression": "true"},
		},
	}

	test, err := newTestModule(t, nil, rulesDef, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      1,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpAutoSuppressionEnabled:  true,
		autoSuppressionEventTypes:           []string{"exec", "dns"},
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

	dockerInstance2, err := test.StartADocker()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance2.stop()

	// dockerInstance2 should not be traced
	_, err = test.GetDumpFromDocker(dockerInstance2)
	if err == nil {
		t.Fatal("second docker instance should not have an active dump")
	}

	time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
	t.Run("auto-suppression-process-suppression", func(t *testing.T) {
		// check we autosuppress signals during the activity dump duration
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance2.Command("getconf", []string{"-a"}, []string{})
			_, err := cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == containerutils.ContainerID(dockerInstance2.containerID) {
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
		// check we autosuppress signals during the activity dump duration
		err = test.GetEventSent(t, func() error {
			cmd := dockerInstance2.Command("nslookup", []string{"foo.bar"}, []string{})
			_, err = cmd.CombinedOutput()
			return err
		}, func(rule *rules.Rule, event *model.Event) bool {
			if event.ProcessContext.ContainerID == containerutils.ContainerID(dockerInstance2.containerID) {
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

}
