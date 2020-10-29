// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestProcess(t *testing.T) {
	currentUser, err := user.LookupId("0")
	if err != nil {
		t.Fatal(err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`process.user == "%s" && process.name == "%s" && open.filename == "{{.Root}}/test-process"`, currentUser.Name, path.Base(executable)),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-process")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, rule, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if rule.ID != "test_rule" {
			t.Errorf("expected rule 'test-rule' to be triggered, got %s", rule.ID)
		}
	}
}

func TestProcessContext(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`open.filename == "{{.Root}}/test-process"`),
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("test-process")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// consume the open creation event
	_, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	}

	t.Run("inode", func(t *testing.T) {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename, _ := event.GetFieldValue("process.filename"); filename.(string) != executable {
				t.Errorf("not able to find the proper process filename `%v` vs `%s`", filename, executable)
			}

			// not working on centos8 in docker env
			/*if inode := getInode(t, executable); inode != event.Process.Inode {
				t.Errorf("expected inode %d, got %d", event.Process.Inode, inode)
			}*/

			testContainerPath(t, event, "process.container_path")
		}
	})

	t.Run("fork", func(t *testing.T) {
		executable := "/usr/bin/cat"
		if _, err := os.Stat(executable); err != nil {
			executable = "/bin/cat"
		}

		cmd := exec.Command("sh", "-c", executable+" "+testFile)
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename, _ := event.GetFieldValue("process.filename"); filename.(string) != executable {
				t.Errorf("not able to find the proper process filename `%v` vs `%s`: %v", filename, executable, event)
			}

			// not working on centos8 in docker env
			/*if inode := getInode(t, executable); inode != event.Process.Inode {
				t.Errorf("expected inode %d, got %d", event.Process.Inode, inode)
			}*/

			testContainerPath(t, event, "process.container_path")
		}
	})

	t.Run("tty", func(t *testing.T) {
		// not working on centos8
		t.Skip()

		executable := "/usr/bin/cat"
		if _, err := os.Stat(executable); err != nil {
			executable = "/bin/cat"
		}

		cmd := exec.Command("script", "/dev/null", "-c", executable+" "+testFile)
		if _, err := cmd.CombinedOutput(); err != nil {
			t.Error(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if filename, _ := event.GetFieldValue("process.filename"); filename.(string) != executable {
				t.Errorf("not able to find the proper process filename `%v` vs `%s`", filename, executable)
			}

			if name, _ := event.GetFieldValue("process.tty_name"); name.(string) == "" {
				t.Error("not able to get a tty name")
			}

			if inode := getInode(t, executable); inode != event.Process.Inode {
				t.Errorf("expected inode %d, got %d", event.Process.Inode, inode)
			}

			testContainerPath(t, event, "process.container_path")
		}
	})
}
