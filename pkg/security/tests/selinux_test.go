// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const TestBoolName = "selinuxuser_ping"
const TestBoolName2 = "httpd_enable_cgi"

func TestSELinux(t *testing.T) {
	ruleset := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_enforce",
			Expression: `selinux.enforce.status in ["disabled", "permissive"]`,
		},
		{
			ID:         "test_selinux_write_bool_true",
			Expression: `selinux.bool.name == "selinuxuser_ping" && selinux.bool.state == "on"`,
		},
		{
			ID:         "test_selinux_write_bool_false",
			Expression: `selinux.bool.name == "selinuxuser_ping" && selinux.bool.state == "off"`,
		},
	}

	if !isSELinuxEnabled() {
		t.Skipf("SELinux is not available, skipping tests")
	}

	// initial setup
	currentEnforceStatus, err := getEnforceStatus()
	if err != nil {
		t.Fatal("failed to save enforce status")
	}
	defer setEnforceStatus(currentEnforceStatus)

	savedBoolValue, err := getBoolValue(TestBoolName)
	if err != nil {
		t.Fatalf("failed to save bool state: %v", err)
	}
	defer setBoolValue(TestBoolName, savedBoolValue)

	test, err := newTestModule(t, nil, ruleset, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setenforce", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := setEnforceStatus("permissive"); err != nil {
				return fmt.Errorf("failed to run setenforce: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_selinux_enforce")
			assert.Equal(t, "selinux", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "selinux.enforce.status", "permissive", "wrong enforce value")

			test.validateSELinuxSchema(t, event)
		})
	})

	t.Run("sel_disable", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := rawSudoWrite("/sys/fs/selinux/disable", "0", false); err != nil {
				return fmt.Errorf("failed to write to selinuxfs: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_selinux_enforce")
			assert.Equal(t, "selinux", event.GetType(), "wrong event type")

			test.validateSELinuxSchema(t, event)
		})
	})

	t.Run("setsebool_true_value", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := setBoolValue(TestBoolName, true); err != nil {
				return fmt.Errorf("failed to run setsebool: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_selinux_write_bool_true")
			assert.Equal(t, "selinux", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "selinux.bool.name", TestBoolName, "wrong bool name")
			assertFieldEqual(t, event, "selinux.bool.state", "on", "wrong bool value")

			test.validateSELinuxSchema(t, event)
		})
	})

	t.Run("setsebool_false_value", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := setBoolValue(TestBoolName, false); err != nil {
				return fmt.Errorf("failed to run setsebool: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_selinux_write_bool_false")
			assert.Equal(t, "selinux", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "selinux.bool.name", TestBoolName, "wrong bool name")
			assertFieldEqual(t, event, "selinux.bool.state", "off", "wrong bool value")

			test.validateSELinuxSchema(t, event)
		})
	})

	t.Run("setsebool_error_value", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			if err := rawSudoWrite("/sys/fs/selinux/booleans/httpd_enable_cgi", "test_error", true); err != nil {
				return fmt.Errorf("failed to write to selinuxfs: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			t.Errorf("expected error and got an event: %s", test.debugEvent(event))
		})
		if err == nil {
			t.Fatal("expected error")
		} else {
			_, ok := err.(ErrTimeout)
			assert.Equal(t, true, ok, "wrong error type, expected ErrTimeout")
		}
	})
}

func TestSELinuxCommitBools(t *testing.T) {
	ruleset := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_commit_bools",
			Expression: `selinux.bool_commit.state == true`,
		},
	}

	test, err := newTestModule(t, nil, ruleset, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if !isSELinuxEnabled() {
		t.Skipf("SELinux is not available, skipping tests")
	}

	savedBoolValue, err := getBoolValue(TestBoolName)
	if err != nil {
		t.Fatalf("failed to save bool state: %v", err)
	}
	defer setBoolValue(TestBoolName, savedBoolValue)

	t.Run("sel_commit_bools", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if err := setBoolValue(TestBoolName, true); err != nil {
				return fmt.Errorf("failed to run setsebool: %w", err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_selinux_commit_bools")
			assert.Equal(t, "selinux", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "selinux.bool_commit.state", true, "wrong bool value")

			test.validateSELinuxSchema(t, event)
		})
	})
}

func rawSudoWrite(filePath, writeContent string, ignoreRunError bool) error {
	cmd := exec.Command("sudo", "-n", "tee", filePath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, writeContent)
	}()

	if err := cmd.Run(); !ignoreRunError {
		return err
	}
	return nil
}

func isSELinuxEnabled() bool {
	cmd := exec.Command("sudo", "-n", "selinuxenabled")
	return cmd.Run() == nil && cmd.ProcessState.Success()
}

func getBoolValue(boolName string) (bool, error) {
	cmd := exec.Command("sudo", "-n", "getsebool", boolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, err
	}

	var name, value string
	fmt.Sscanf(string(output), "%s --> %s", &name, &value)

	if name != boolName {
		return false, errors.New("bool name mismatch")
	}

	switch value {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("unknown bool representation: %v", value)
	}
}

func setBoolValue(boolName string, value bool) error {
	var valueStr string
	if value {
		valueStr = "on"
	} else {
		valueStr = "off"
	}

	cmd := exec.Command("sudo", "-n", "setsebool", boolName, valueStr)
	return cmd.Run()
}

func getEnforceStatus() (string, error) {
	cmd := exec.Command("sudo", "-n", "getenforce")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	status := strings.ToLower(strings.TrimSpace(string(output)))
	return status, nil
}

func setEnforceStatus(status string) error {
	enforceNumber := 0
	switch status {
	case "enforcing":
		enforceNumber = 1
	case "permissive":
		enforceNumber = 0
	case "disabled":
		return nil
	default:
		return nil
	}

	cmd := exec.Command("sudo", "-n", "setenforce", strconv.Itoa(enforceNumber))
	_, err := cmd.Output()
	return err
}
