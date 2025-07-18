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
	"runtime"
	"testing"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
)

func TestEnv(t *testing.T) {
	if testEnvironment != "" && testEnvironment != HostEnvironment && testEnvironment != DockerEnvironment {
		t.Error("invalid environment")
		return
	}
}

func TestOsOrigin(t *testing.T) {
	SkipIfNotAvailable(t)

	origin := "ebpf"
	if ebpfLessEnabled {
		origin = "ebpfless"
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_origin",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-origin" && event.origin == "%s" && event.os == "%s"`, origin, runtime.GOOS),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create("test-origin")
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(_ *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_origin")
	})
}

func TestHostname(t *testing.T) {
	SkipIfNotAvailable(t)

	ipcComp := ipcmock.New(t)

	hostname, err := hostnameutils.GetHostname(ipcComp)
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	ruleDef := &rules.RuleDefinition{
		ID:         "test_hostname",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/test-hostname" && event.hostname == "%s"`, hostname),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	test.WaitSignal(t, func() error {
		testFile, _, err := test.Create("test-hostname")
		if err != nil {
			return err
		}
		return os.Remove(testFile)
	}, func(_ *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_hostname")
	})
}
