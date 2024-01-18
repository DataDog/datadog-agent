// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	activitydump "github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
)

func TestActivityDumpsThreatScore(t *testing.T) {
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

	expectedFormats := []string{"json", "protobuf"}
	testActivityDumpTracedEventTypes := []string{"exec", "open", "syscalls", "dns", "bind"}
	test, err := newTestModule(t, nil, rules, withStaticOpts(testOpts{
		enableActivityDump:                  true,
		activityDumpRateLimiter:             testActivityDumpRateLimiter,
		activityDumpTracedCgroupsCount:      testActivityDumpTracedCgroupsCount,
		activityDumpDuration:                testActivityDumpDuration,
		activityDumpLocalStorageDirectory:   outputDir,
		activityDumpLocalStorageCompression: false,
		activityDumpLocalStorageFormats:     expectedFormats,
		activityDumpTracedEventTypes:        testActivityDumpTracedEventTypes,
		activityDumpTagRules:                true,
	}))
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
		time.Sleep(time.Second * 1) // to ensure we did not get ratelimited
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
			nodes := ad.ActivityTree.FindMatchingRootNodes("touch")
			if nodes == nil || len(nodes) != 1 {
				t.Fatal("Uniq node not found in activity dump")
			}
			node := nodes[0]

			var next *activity_tree.FileNode
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
		}, nil)
	})

	t.Run("activity-dump-tag-rule-dns", func(t *testing.T) {
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.ActivityTree.FindMatchingRootNodes("nslookup")
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
		}, nil)
	})

	t.Run("activity-dump-tag-rule-bind", func(t *testing.T) {
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

		err = test.StopActivityDump(dump.Name, "", "")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.ActivityTree.FindMatchingRootNodes(syscallTester)
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
							}
							return true
						}
					}
				}
			}
			return false
		}, nil)
	})

	t.Run("activity-dump-tag-rule-process", func(t *testing.T) {
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

		validateActivityDumpOutputs(t, test, expectedFormats, dump.OutputFiles, func(ad *activitydump.ActivityDump) bool {
			nodes := ad.ActivityTree.FindMatchingRootNodes(syscallTester)
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
		}, nil)
	})

}
