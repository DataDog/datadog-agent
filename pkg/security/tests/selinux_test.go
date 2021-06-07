// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

func TestSELinux(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux",
			Expression: `selinux.magic == 42`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if cmd := exec.Command("sudo", "-n", "sestatus"); cmd.Run() != nil {
		t.Skipf("SELinux is not available, skipping tests")
	}

	t.Run("setenforce", func(t *testing.T) {
		if cmd := exec.Command("sudo", "-n", "setenforce", "0"); cmd.Run() != nil {
			t.Errorf("Failed to run setenforce")
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux")
			assert.Equal(t, event.SELinux.Magic, uint32(42), "wrong magic")
		}
	})

	t.Run("setsebool", func(t *testing.T) {
		if cmd := exec.Command("sudo", "-n", "setsebool", "selinuxuser_ping", "off"); cmd.Run() != nil {
			t.Errorf("Failed to run setsebool")
		}

		event, rule, err := test.GetEvent()
		t.Log(ppJSON(event))
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux")
			assert.Equal(t, event.SELinux.Magic, uint32(42), "wrong magic")
		}

		if cmd := exec.Command("sudo", "-n", "setsebool", "selinuxuser_ping", "on"); cmd.Run() != nil {
			t.Errorf("Failed to reset setsebool value")
		}
	})
}
