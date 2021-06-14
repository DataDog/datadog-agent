// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type SELinuxResolver struct {
}

// GetCurrentBoolValue returns the current value of the provided SELinux boolean
func (r *SELinuxResolver) GetCurrentBoolValue(boolName string) (bool, error) {
	output, err := exec.Command("getsebool", boolName).Output()
	if err != nil {
		return false, err
	}

	var resName, boolValue string
	parsed, err := fmt.Sscanf(string(output), "%s --> %s", &resName, &boolValue)
	if err != nil {
		return false, err
	}

	if parsed != 2 || resName != boolName || (boolValue != "on" && boolValue != "off") {
		return false, errors.New("failed to parse getsebool output")
	}

	return boolValue == "on", nil
}

// GetCurrentEnforceStatus returns the current SELinux enforcement status, one of "enforcing", "permissive", "disabled"
func (r *SELinuxResolver) GetCurrentEnforceStatus() (string, error) {
	output, err := exec.Command("getenforce").Output()
	if err != nil {
		return "", err
	}

	status := strings.ToLower(strings.TrimSpace(string(output)))
	switch status {
	case "enforcing", "permissive", "disabled":
		return status, nil
	default:
		return "", errors.New("failed to parse getenforce output")
	}
}
