// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activitydump "github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"

	"github.com/stretchr/testify/assert"
)

// v see test/kitchen/test/integration/security-agent-test/rspec/security-agent-test_spec.rb
const dedicatedADNodeForTestsEnv = "DEDICATED_ACTIVITY_DUMP_NODE"

var expectedFormats = []string{"json", "protobuf"}

const testActivityDumpRateLimiter = 20
const testActivityDumpTracedCgroupsCount = 3
const testActivityDumpCgroupDumpTimeout = 11 // probe.MinDumpTimeout(10) + 5
var testActivityDumpTracedEventTypes = []string{"exec", "open", "syscalls", "dns", "bind"}

func TestActivityDumps(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("activity-dump-cgroup-process", func(t *testing.T) {
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
			if nodes == nil {
				t.Fatalf("Node not found in activity dump: %+v", nodes)
			}
			if len(nodes) != 1 {
				t.Fatalf("Found %d nodes, expected only one.", len(nodes))
			}
			node := nodes[0]

			// ProcessActivityNode content1
			assert.Equal(t, activitydump.Runtime, node.GenerationType)
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
			assert.Equal(t, false, node.Process.IsThread)
			assert.Equal(t, uint64(0), node.Process.SpanID)
			assert.Equal(t, uint64(0), node.Process.TraceID)
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
		})
	})

	t.Run("activity-dump-cgroup-bind", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
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
		})
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("nslookup")
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
		})
	})

	t.Run("activity-dump-cgroup-file", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		temp, _ := os.CreateTemp(test.st.Root(), "ad-test-create")
		os.Remove(temp.Name()) // next touch command have to create the file

		cmd := dockerInstance.Command("touch", []string{temp.Name()}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		tempPathParts := strings.Split(temp.Name(), "/")
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("touch")
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
		})
	})

	t.Run("activity-dump-cgroup-syscalls", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			var exitOK, execveOK bool
			for _, node := range nodes {
				for _, s := range node.Syscalls {
					if s == int(model.SysExit) || s == int(model.SysExitGroup) {
						exitOK = true
					}
					if s == int(model.SysExecve) || s == int(model.SysExecveat) {
						execveOK = true
					}
				}
			}
			if !exitOK {
				t.Errorf("exit syscall not found in activity dump")
			}
			if !execveOK {
				t.Errorf("execve syscall not found in activity dump")
			}
			return exitOK && execveOK
		})
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
		cmd := dockerInstance.Command(syscallTester, args, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
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
		})
	})

	t.Run("activity-dump-cgroup-timeout", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		// check that the dump is still alive
		time.Sleep((testActivityDumpCgroupDumpTimeout*60 - 20) * time.Second)
		assert.Equal(t, true, test.isDumpRunning(dump))

		// check that the dump has timeouted after the cleanup period (30s) + 2s
		time.Sleep(1 * time.Minute)
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

func TestActivityDumpsThreatScore(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	rules := []*rules.RuleDefinition{
		{
			ID:         "tag_rule_threat_score_file",
			Expression: `open.file.name == "tag-open" && process.file.name == "touch"`,
			Tags:       map[string]string{"ruleset": "threat_score"},
			Version:    "4.5.6",
		},
		{
			ID:         "tag_rule_threat_score_dns",
			Expression: `dns.question.name == "foo.bar" && process.file.name == "nslookup"`,
			Tags:       map[string]string{"ruleset": "threat_score"},
			Version:    "4.5.6",
		},
		{
			ID:         "tag_rule_threat_score_bind",
			Expression: `bind.addr.family == AF_INET && process.file.name == "syscall_tester"`,
			Tags:       map[string]string{"ruleset": "threat_score"},
			Version:    "4.5.6",
		},
		{
			ID:         "tag_rule_threat_score_process",
			Expression: `exec.file.name == "syscall_tester"`,
			Tags:       map[string]string{"ruleset": "threat_score"},
			Version:    "4.5.6",
		},
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, rules, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpTagRules:                true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("activity-dump-tag-rule-file", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		filePath := filepath.Join(test.st.Root(), "tag-open")
		cmd := dockerInstance.Command("touch", []string{filePath}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		tempPathParts := strings.Split(filePath, "/")
		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("touch")
			if nodes == nil || len(nodes) != 1 {
				t.Fatal("Uniq node not found in activity dump")
			}
			node := nodes[0]

			var next *activitydump.FileActivityNode
			var found bool
			current := node.Files
			for _, part := range tempPathParts {
				if part == "" {
					continue
				}
				next, found = current[part]
				if !found {
					return false
				}
				current = next.Children
			}
			if next == nil || len(next.MatchedRules) != 1 ||
				next.MatchedRules[0].RuleID != "tag_rule_threat_score_file" ||
				next.MatchedRules[0].RuleVersion != "4.5.6" {
				return false
			}
			return true
		})
	})

	t.Run("activity-dump-tag-rule-dns", func(t *testing.T) {
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("nslookup")
			if nodes == nil || len(nodes) != 1 {
				t.Fatal("Uniq node not found in activity dump")
			}
			node := nodes[0]

			for dnsName, dnsReq := range node.DNSNames {
				if dnsName == "foo.bar" {
					if len(dnsReq.MatchedRules) != 1 ||
						dnsReq.MatchedRules[0].RuleID != "tag_rule_threat_score_dns" ||
						dnsReq.MatchedRules[0].RuleVersion != "4.5.6" {
						return false
					}
					return true
				}
			}
			return false
		})
	})

	t.Run("activity-dump-tag-rule-bind", func(t *testing.T) {
		dockerInstance, dump, err := test.StartADockerGetDump()
		if err != nil {
			t.Fatal(err)
		}
		defer dockerInstance.stop()

		cmd := dockerInstance.Command(syscallTester, []string{"bind", "AF_INET", "any", "tcp"}, []string{})
		_, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
			if nodes == nil || len(nodes) != 1 {
				t.Fatal("Uniq node not found in activity dump")
			}
			node := nodes[0]

			for _, s := range node.Sockets {
				if s.Family == "AF_INET" {
					for _, bindNode := range s.Bind {
						if bindNode.Port == 4242 && bindNode.IP == "0.0.0.0" {
							if len(bindNode.MatchedRules) != 1 ||
								bindNode.MatchedRules[0].RuleID != "tag_rule_threat_score_bind" ||
								bindNode.MatchedRules[0].RuleVersion != "4.5.6" {
								return false
							} else {
								return true
							}
						}
					}
				}
			}
			return false
		})
	})

	t.Run("activity-dump-tag-rule-process", func(t *testing.T) {
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.FindMatchingNodes("syscall_tester")
			if nodes == nil {
				t.Fatal("Node not found in activity dump")
			}
			for _, node := range nodes {
				if len(node.MatchedRules) == 1 &&
					node.MatchedRules[0].RuleID == "tag_rule_threat_score_process" &&
					node.MatchedRules[0].RuleVersion == "4.5.6" {
					return true
				}
			}
			return false
		})
	})

}

func validateActivityDumpOutputs(t *testing.T, test *testModule, expectedFormats []string, outputFiles []string, validator func(ad *activitydump.ActivityDump) bool) {
	perExtOK := make(map[string]bool)
	for _, format := range expectedFormats {
		ext := fmt.Sprintf(".%s", format)
		perExtOK[ext] = false
	}

	for _, f := range outputFiles {
		ext := filepath.Ext(f)
		if perExtOK[ext] {
			t.Fatalf("Got more than one `%s` file: %v", ext, outputFiles)
		}

		switch ext {
		case ".json":
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if !validateActivityDumpProtoSchema(t, string(content)) {
				t.Error(string(content))
			}
			perExtOK[ext] = true

		case ".protobuf":
			ad, err := test.DecodeActivityDump(f)
			if err != nil {
				t.Fatal(err)
			}

			found := validator(ad)
			if !found {
				t.Error("Invalid activity dump")
			}
			perExtOK[ext] = found

		default:
			t.Fatal("Unexpected output file")
		}
	}

	for ext, found := range perExtOK {
		if !found {
			t.Fatalf("Missing or wrong `%s`, out of: %v", ext, outputFiles)
		}
	}
}

func TestActivityDumpsLoadControllerTimeout(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()
	assert.Equal(t, "11m0s", dump.Timeout)

	// trigg reducer (before t > timeout / 4)
	test.triggerLoadControlerReducer(dockerInstance, dump)

	// find the new dump, with timeout *= 3/4, or min timeout
	secondDump, err := test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "10m0s", secondDump.Timeout)
}

func TestActivityDumpsLoadControllerEventTypes(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	for activeEventTypes := activitydump.TracedEventTypesReductionOrder; ; activeEventTypes = activeEventTypes[1:] {
		// add all event types to the dump
		test.addAllEventTypesOnDump(dockerInstance, dump, syscallTester)
		time.Sleep(time.Second * 3)
		// trigg reducer
		test.triggerLoadControlerReducer(dockerInstance, dump)
		// find the new dump
		nextDump, err := test.findNextPartialDump(dockerInstance, dump)
		if err != nil {
			t.Fatal(err)
		}

		// extract all present event types present on the first dump
		presentEventTypes, err := test.extractAllDumpEventTypes(dump)
		if err != nil {
			t.Fatal(err)
		}
		if !isEventTypesStringSlicesEqual(activeEventTypes, presentEventTypes) {
			t.Fatalf("Dump's event types are different as expected (%v) vs (%v)", activeEventTypes, presentEventTypes)
		}
		if len(activeEventTypes) == 0 {
			break
		}
		dump = nextDump
	}
}

func TestActivityDumpsLoadControllerRateLimiter(t *testing.T) {
	// skip test that are about to be run on docker (to avoid trying spawning docker in docker)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}
	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}
	if !IsDedicatedNode(dedicatedADNodeForTestsEnv) {
		t.Skip("Skip test when not run in dedicated env")
	}

	outputDir := t.TempDir()
	defer os.RemoveAll(outputDir)
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpCgroupDumpTimeout:       testActivityDumpCgroupDumpTimeout,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	// first, stop all running activity dumps
	err = test.StopAllActivityDumps()
	if err != nil {
		t.Fatal("Can't stop all running activity dumps")
	}

	dockerInstance, dump, err := test.StartADockerGetDump()
	if err != nil {
		t.Fatal(err)
	}
	defer dockerInstance.stop()

	// burst file creation
	testDir := filepath.Join(test.Root(), "ratelimiter")
	os.MkdirAll(testDir, os.ModePerm)
	test.dockerCreateFiles(dockerInstance, syscallTester, testDir, testActivityDumpRateLimiter*2)
	time.Sleep(time.Second * 3)
	// trigg reducer
	test.triggerLoadControlerReducer(dockerInstance, dump)
	// find the new dump, with ratelimiter *= 3/4
	secondDump, err := test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}

	// find the number of files creation that were added to the dump
	numberOfFiles, err := test.findNumberOfExistingDirectoryFiles(dump, testDir)
	if err != nil {
		t.Fatal(err)
	}
	if numberOfFiles < testActivityDumpRateLimiter/4 || numberOfFiles > testActivityDumpRateLimiter {
		t.Fatalf("number of files not expected (%d) with %d ratelimiter\n", numberOfFiles, testActivityDumpRateLimiter)
	}

	dump = secondDump
	// burst file creation
	test.dockerCreateFiles(dockerInstance, syscallTester, testDir, testActivityDumpRateLimiter*2)
	time.Sleep(time.Second * 3)
	// trigg reducer
	test.triggerLoadControlerReducer(dockerInstance, dump)
	// find the new dump, with ratelimiter *= 3/4
	_, err = test.findNextPartialDump(dockerInstance, dump)
	if err != nil {
		t.Fatal(err)
	}
	// find the number of files creation that were added to the dump
	numberOfFiles, err = test.findNumberOfExistingDirectoryFiles(dump, testDir)
	if err != nil {
		t.Fatal(err)
	}
	newRateLimiter := testActivityDumpRateLimiter / 4 * 3
	if numberOfFiles < newRateLimiter/4 || numberOfFiles > newRateLimiter {
		t.Fatalf("number of files not expected (%d) with %d ratelimiter\n", numberOfFiles, newRateLimiter)
	}
}

func isEventTypesStringSlicesEqual(slice1 []model.EventType, slice2 []string) bool {
	if len(slice1) != len(slice2) {
		return false
	}
firstLoop:
	for _, s1 := range slice1 {
		for _, s2 := range slice2 {
			if s1.String() == s2 {
				continue firstLoop
			}
		}
		return false
	}
	return true
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
