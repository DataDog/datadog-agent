// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests,!386

package tests

import (
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

func TestChown32(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	ruleDef2 := &rules.RuleDefinition{
		ID:         "test_rule2",
		Expression: `chown.file.path == "{{.Root}}/test-symlink" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef, ruleDef2}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t)
	if err != nil {
		t.Fatal(err)
	}

	prevUID := 98
	prevGID := 99
	fileMode := 0o447
	expectedMode := uint32(applyUmask(fileMode))
	testFile, _, err := test.CreateWithOptions("test-chown", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("chown32", func(t *testing.T) {
		sideTester := exec.Command(syscallTester, "chown32", testFile, "100", "200")
		if output, err := sideTester.CombinedOutput(); err != nil {
			t.Error(string(output))
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "chown", "wrong event type")
			assert.Equal(t, event.Chown.UID, uint32(100), "wrong user")
			assert.Equal(t, event.Chown.GID, uint32(200), "wrong user")
			assert.Equal(t, event.Chown.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, event.Chown.File.UID, uint32(prevUID), "wrong initial user")
			assert.Equal(t, event.Chown.File.GID, uint32(prevGID), "wrong initial group")

			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "chown.file.container_path")
			}

			if !validateChownSchema(t, event) {
				t.Fatal(event.String())
			}
		}
	})
}
