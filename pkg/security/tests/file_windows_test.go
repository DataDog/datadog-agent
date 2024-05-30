// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"os/exec"
	"testing"
	"time"

	// include the below to activate logging in tests.
	_ "github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestBasicFileTest(t *testing.T) {
	//ebpftest.LogLevel(t, "info")
	cfn := &rules.RuleDefinition{
		ID:         "test_create_file",
		Expression: `create.file.name =~ "test.bad" && create.file.path =~ "C:\Temp\**"`,
	}
	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{cfn}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	test.Run(t, "File test 1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {

		os.MkdirAll("C:\\Temp", 0755)

		// ignore errors; just clean it out if it's there
		os.Remove("C:\\Temp\\test.bad")
		inputargs := []string{
			"-c",
			"New-Item",
			"-Path",
			"C:\\Temp\\test.bad",
			"-ItemType",
			"file",
		}
		test.WaitSignal(t, func() error {
			cmd := cmdFunc("powershell", inputargs, nil)
			_ = cmd.Run()
			return nil
		}, test.validateFileEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "create.file.name", "test.bad", event, "create.file.name file didn't match")
		}))
	})

}

func TestRenameFileEvent(t *testing.T) {
	// ebpftest.LogLevel(t, "info")
	cfn := &rules.RuleDefinition{
		ID:         "test_rename_file",
		Expression: `rename.file.name =~ "test.bad" && rename.file.path =~ "C:\Temp\**"`,
	}
	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{cfn}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	os.MkdirAll("C:\\Temp", 0755)
	f, err := os.Create("C:\\Temp\\test.bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	test.Run(t, "rename", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			return os.Rename("C:\\Temp\\test.bad", "C:\\Temp\\test.good")
		}, test.validateFileEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "rename.file.name", "test.bad", event, "rename.file.name file didn't match")
			assertFieldEqualCaseInsensitve(t, event, "rename.file.destination.name", "test.good", event, "rename.file.destination.name file didn't match")
		}))
	})
}

func TestDeleteFileEvent(t *testing.T) {
	// ebpftest.LogLevel(t, "info")
	cfn := &rules.RuleDefinition{
		ID:         "test_delete_file",
		Expression: `delete.file.name =~ "test.bad" && delete.file.path =~ "C:\Temp\**"`,
	}
	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{cfn}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	os.MkdirAll("C:\\Temp", 0755)
	f, err := os.Create("C:\\Temp\\test.bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	test.Run(t, "delete", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			return os.Remove("C:\\Temp\\test.bad")
		}, test.validateFileEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "delete.file.name", "test.bad", event, "delete.file.name file didn't match")
		}))
	})
}

func TestWriteFileEvent(t *testing.T) {
	// ebpftest.LogLevel(t, "info")
	cfn := &rules.RuleDefinition{
		ID:         "test_write_file",
		Expression: `write.file.name =~ "test.bad" && write.file.path =~ "C:\Temp\**"`,
	}
	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{cfn}, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	os.MkdirAll("C:\\Temp", 0755)
	f, err := os.Create("C:\\Temp\\test.bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	test.Run(t, "write", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			f, err := os.OpenFile("C:\\Temp\\test.bad", os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			if _, err := f.WriteString("test"); err != nil {
				return err
			}
			return f.Close()
		}, test.validateFileEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "write.file.name", "test.bad", event, "write.file.name file didn't match")
		}))
	})
}

func TestWriteFileEventWithCreate(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_create_polute",
			Expression: `create.file.name =~ "*.dll"`,
		},
		{
			ID:         "test_write_file",
			Expression: `write.file.name =~ "test.bad" && write.file.path =~ "C:\Temp\**"`,
		},
	}
	opts := testOpts{
		enableFIM: true,
	}
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()
	// this is kinda hokey.  ETW (which is what FIM is based on) takes an indeterminant amount of time to start up.
	// so wait around for it to start
	time.Sleep(5 * time.Second)

	os.MkdirAll("C:\\Temp", 0755)
	f, err := os.Create("C:\\Temp\\test.bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	test.Run(t, "write", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		test.WaitSignal(t, func() error {
			f, err := os.OpenFile("C:\\Temp\\test.bad", os.O_WRONLY, 0755)
			if err != nil {
				return err
			}
			if _, err := f.WriteString("test"); err != nil {
				return err
			}
			return f.Close()
		}, test.validateFileEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "write.file.name", "test.bad", event, "write.file.name file didn't match")
		}))
	})
}

func (tm *testModule) validateFileEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
	}
}
