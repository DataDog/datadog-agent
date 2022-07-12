// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestActivityDumps(t *testing.T) {
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{}, testOpts{enableActivityDump: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	outputDir, _, err := test.Path("test-activity-dump")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	test.Run(t, "activity-dump-comm-bind", func(t *testing.T, kind wrapperType,
		cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {

		expectedFormats := []string{"json", "msgp"}
		outputFiles, err := test.StartActivityDumpComm(t, "syscall_tester", outputDir, expectedFormats)
		if err != nil {
			t.Fatal(err)
		}

		args := []string{"bind", "AF_INET", "any", "tcp"}
		envs := []string{}
		cmd := cmdFunc(syscallTester, args, envs)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(fmt.Errorf("%s: %w", out, err))
		}

		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDumpComm(t, "syscall_tester")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, outputFiles, func(ad *probe.ActivityDump) bool {
			node := ad.FindFirstMatchingNode("syscall_tester")
			if node == nil {
				t.Fatal("Node not found on activity dump")
			}
			for _, s := range node.Sockets {
				if s.Family == "AF_INET" && s.Bind.Port == 4242 && s.Bind.IP == "0.0.0.0" {
					return true
				}
			}
			return false
		})
	})

	test.Run(t, "activity-dump-comm-dns", func(t *testing.T, kind wrapperType,
		cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {

		expectedFormats := []string{"json", "msgp"}
		outputFiles, err := test.StartActivityDumpComm(t, "testsuite", outputDir, expectedFormats)
		if err != nil {
			t.Fatal(err)
		}

		net.LookupIP("foo.bar")

		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDumpComm(t, "testsuite")
		if err != nil {
			t.Fatal(err)
		}

		validateActivityDumpOutputs(t, test, expectedFormats, outputFiles, func(ad *probe.ActivityDump) bool {
			node := ad.FindFirstMatchingNode("testsuite")
			if node == nil {
				t.Fatal("Node not found on activity dump")
			}
			for name := range node.DNSNames {
				if name == "foo.bar" {
					return true
				}
			}
			return false
		})
	})

	test.Run(t, "activity-dump-comm-file", func(t *testing.T, kind wrapperType,
		cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {

		expectedFormats := []string{"json", "msgp"}
		outputFiles, err := test.StartActivityDumpComm(t, "testsuite", outputDir, expectedFormats)
		if err != nil {
			t.Fatal(err)
		}

		temp, err := os.CreateTemp("", "ad-test-create")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(temp.Name())

		time.Sleep(1 * time.Second) // a quick sleep to let events to be added to the dump

		err = test.StopActivityDumpComm(t, "testsuite")
		if err != nil {
			t.Fatal(err)
		}

		tempPathParts := strings.Split(temp.Name(), "/")

		validateActivityDumpOutputs(t, test, expectedFormats, outputFiles, func(ad *probe.ActivityDump) bool {
			node := ad.FindFirstMatchingNode("testsuite")
			if node == nil {
				t.Fatal("Node not found on activity dump")
			}

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

			return true
		})
	})
}

func validateActivityDumpOutputs(t *testing.T, test *testModule, expectedFormats []string, outputFiles []string, msgpValidator func(ad *probe.ActivityDump) bool) {
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
			if !validateActivityDumpSchema(t, string(content)) {
				t.Error(string(content))
			}
			perExtOK[ext] = true

		case ".msgp":
			ad, err := test.DecodeMSPActivityDump(t, f)
			if err != nil {
				t.Fatal(err)
			}

			found := msgpValidator(ad)
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
			t.Fatalf("Missing `%s`, got: %v", ext, outputFiles)
		}
	}
}
