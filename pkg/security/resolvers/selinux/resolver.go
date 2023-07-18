// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package selinux

import (
	"os/exec"
	"strings"

	"github.com/cilium/ebpf"

	sebpf "github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

const (
	// SELinuxStatusDisableKey represents the key in the kernel map managing the current SELinux disable status
	SELinuxStatusDisableKey uint32 = 0
	// SELinuxStatusEnforceKey represents the key in the kernel map managing the current SELinux enforce status
	SELinuxStatusEnforceKey uint32 = 1
)

func SnapshotSELinux(selinuxStatusMap *ebpf.Map) error {
	currentStatus := func() string {
		output, err := exec.Command("getenforce").Output()
		if err != nil {
			return ""
		}

		status := strings.ToLower(strings.TrimSpace(string(output)))
		switch status {
		case "enforcing", "permissive", "disabled":
			return status
		default:
			return ""
		}
	}()

	var disableValue, enforceValue uint16
	switch currentStatus {
	case "disabled":
		disableValue = 1
		enforceValue = 0
	case "enforcing":
		disableValue = 0
		enforceValue = 1
	case "permissive":
	default:
		disableValue = 0
		enforceValue = 0
	}

	if err := selinuxStatusMap.Update(sebpf.Uint32MapItem(SELinuxStatusDisableKey), sebpf.Uint16MapItem(disableValue), ebpf.UpdateAny); err != nil {
		return err
	}

	return selinuxStatusMap.Update(sebpf.Uint32MapItem(SELinuxStatusEnforceKey), sebpf.Uint16MapItem(enforceValue), ebpf.UpdateAny)
}
