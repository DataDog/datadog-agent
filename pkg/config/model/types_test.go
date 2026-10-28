// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "testing"

func TestViolationWarning(t *testing.T) {
	for _, tc := range []struct {
		name       string
		message    string
		settingKey string
		reverted   bool
		want       string
	}{
		{"reverted", "at '/hostname': invalid", "hostname", true,
			"invalid value for 'hostname', reverted to default. Error: `at '/hostname': invalid`"},
		{"kept", "at '/hostname': invalid", "hostname", false,
			"invalid value for 'hostname'. Error: `at '/hostname': invalid`"},
		{"no setting key", "invalid root", "", true, "invalid configuration: `invalid root`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violation := &Violation{Message: tc.message, SettingKey: tc.settingKey}
			if tc.reverted {
				violation.MarkAsReverted()
			}
			if got := violation.Warning(); got != tc.want {
				t.Errorf("Warning() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestViolationMarkAsReverted(t *testing.T) {
	violation := &Violation{}
	if violation.WasReverted() {
		t.Error("a new violation must not be reverted")
	}
	violation.MarkAsReverted()
	if !violation.WasReverted() {
		t.Error("the violation must be reverted once marked")
	}
}
