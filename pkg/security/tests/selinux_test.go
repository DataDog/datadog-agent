// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

const TEST_BOOL_NAME = "selinuxuser_ping"
const TEST_BOOL_NAME2 = "httpd_enable_cgi"

func TestSELinux(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_enforce",
			Expression: `selinux.enforce.status in ["enabled", "permissive"]`,
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

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	if cmd := exec.Command("sudo", "-n", "sestatus"); cmd.Run() != nil {
		t.Skipf("SELinux is not available, skipping tests")
	}

	savedBoolValue, err := getBoolValue(TEST_BOOL_NAME)
	if err != nil {
		t.Errorf("failed to save bool state: %v", err)
	}
	defer setBoolValue(TEST_BOOL_NAME, savedBoolValue)

	t.Run("sel_disable", func(t *testing.T) {
		if err := rawSudoWrite("/sys/fs/selinux/disable", "0", false); err != nil {
			t.Errorf("failed to write to selinuxfs: %v", err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_enforce")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})

	t.Run("setenforce", func(t *testing.T) {
		if cmd := exec.Command("sudo", "-n", "setenforce", "0"); cmd.Run() != nil {
			t.Errorf("Failed to run setenforce")
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_enforce")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.enforce.status", "permissive", "wrong enforce value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})

	t.Run("setsebool_true_value", func(t *testing.T) {
		if err := setBoolValue(TEST_BOOL_NAME, true); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_write_bool_true")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.bool.name", TEST_BOOL_NAME, "wrong bool name")
			assertFieldEqual(t, event, "selinux.bool.state", "on", "wrong bool value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})

	t.Run("setsebool_false_value", func(t *testing.T) {
		if err := setBoolValue(TEST_BOOL_NAME, false); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_write_bool_false")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.bool.name", TEST_BOOL_NAME, "wrong bool name")
			assertFieldEqual(t, event, "selinux.bool.state", "off", "wrong bool value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})

	t.Run("setsebool_error_value", func(t *testing.T) {
		if err := rawSudoWrite("/sys/fs/selinux/booleans/httpd_enable_cgi", "test_error", true); err != nil {
			t.Errorf("failed to write to selinuxfs: %v", err)
		}

		_, _, err = test.GetEventWithTimeout(1 * time.Second)
		assert.Equal(t, err.Error(), "timeout", "wrong error type, expected timeout")
	})
}

func TestSELinuxCommitBools(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_commit_bools",
			Expression: `selinux.bool_commit.state == true`,
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

	savedBoolValue, err := getBoolValue(TEST_BOOL_NAME)
	if err != nil {
		t.Errorf("failed to save bool state: %v", err)
	}
	defer setBoolValue(TEST_BOOL_NAME, savedBoolValue)

	t.Run("sel_commit_bools", func(t *testing.T) {
		if err := setBoolValue(TEST_BOOL_NAME, true); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_commit_bools")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.bool_commit.state", true, "wrong bool value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})
}

func TestSELinuxBoolChangeBasic(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_bool_has_changed",
			Expression: `selinux.bool.name == "httpd_enable_cgi" && selinux.bool.changed == true`,
		},
		{
			ID:         "test_selinux_bool_has_not_changed",
			Expression: `selinux.bool.name == "httpd_enable_cgi" && selinux.bool.changed == false`,
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

	startBoolValue, err := getBoolValue(TEST_BOOL_NAME2)
	if err != nil {
		t.Errorf("failed to save bool state: %v", err)
	}
	defer setBoolValue(TEST_BOOL_NAME2, startBoolValue)

	t.Run("sel_bool_change_value", func(t *testing.T) {
		// init phase
		if err := setBoolValue(TEST_BOOL_NAME2, startBoolValue); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_bool_has_changed")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			// always true at first
			assertFieldEqual(t, event, "selinux.bool.changed", true, "wrong bool change value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}

		// no change phase
		if err := setBoolValue(TEST_BOOL_NAME2, startBoolValue); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_bool_has_not_changed")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			// always true at first
			assertFieldEqual(t, event, "selinux.bool.changed", false, "wrong bool change value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}

		// actual change phase
		if err := setBoolValue(TEST_BOOL_NAME2, !startBoolValue); err != nil {
			t.Errorf("failed to run setsebool: %v", err)
		}

		event, rule, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_bool_has_changed")
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.bool.changed", true, "wrong bool change value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
	})
}

func TestSELinuxEnforceStatusChange(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_selinux_status_has_changed",
			Expression: `selinux.enforce.status == "permissive" && selinux.enforce.changed == true`,
		},
		{
			ID:         "test_selinux_status_has_not_changed",
			Expression: `selinux.enforce.status == "permissive" && selinux.enforce.changed == false`,
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

	t.Run("setenforce_changed_value", func(t *testing.T) {
		// go to permissive mode
		if cmd := exec.Command("sudo", "-n", "setenforce", "0"); cmd.Run() != nil {
			t.Errorf("Failed to run setenforce")
		}

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_status_has_changed") // first time, so changed = true
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.enforce.status", "permissive", "wrong enforce value")
			assertFieldEqual(t, event, "selinux.enforce.changed", true, "wrong changed value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}

		// go AGAIN to permissive mode
		if cmd := exec.Command("sudo", "-n", "setenforce", "0"); cmd.Run() != nil {
			t.Errorf("Failed to run setenforce")
		}

		event, rule, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_selinux_status_has_not_changed") // seconc time, so changed = false
			assert.Equal(t, event.GetType(), "selinux", "wrong event type")

			assertFieldEqual(t, event, "selinux.enforce.status", "permissive", "wrong enforce value")
			assertFieldEqual(t, event, "selinux.enforce.changed", false, "wrong changed value")

			if !validateSELinuxSchema(t, event) {
				t.Fatal(event.String())
			}

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rename.file.container_path")
				testContainerPath(t, event, "rename.file.destination.container_path")
			}
		}
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
	} else {
		return nil
	}
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
