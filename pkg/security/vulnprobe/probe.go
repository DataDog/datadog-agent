// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package vulnprobe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
)

const UprobeUID = "vuln_detector"
const UprobeSection = "uprobe/vuln_detector"
const UprobeFuncName = "uprobe_vuln_detector"
const UprobeConstantName = "vuln_id"

var uprobeCpt uint64 = 0

func AttachProbe(m *manager.Manager, path, funcName string) (uint64, string, error) {
	uprobeCpt++
	uid := fmt.Sprintf("vuln_detector_%d", uprobeCpt)
	pID := manager.ProbeIdentificationPair{
		UID:          uid,
		EBPFSection:  UprobeSection,
		EBPFFuncName: UprobeFuncName,
	}
	p := &manager.Probe{
		ProbeIdentificationPair: pID,
		BinaryPath:              path,
		HookFuncName:            funcName,
		CopyProgram:             true,
		Enabled:                 true,
	}
	ce := manager.ConstantEditor{
		Name:                     UprobeConstantName,
		Value:                    uint64(uprobeCpt),
		ProbeIdentificationPairs: []manager.ProbeIdentificationPair{pID},
	}
	err := m.CloneProgram(UprobeUID, p, []manager.ConstantEditor{ce}, nil)
	if err != nil {
		return 0, "", err
	}

	seclog.Infof("attached %s:%s with UID %s and vuln_id %d\n", path, funcName, uid, uprobeCpt)
	return uprobeCpt, uid, nil
}

func GetVulncheckProbe() *manager.Probe {
	return &manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          UprobeUID,
			EBPFSection:  UprobeSection,
			EBPFFuncName: UprobeFuncName,
		},
		BinaryPath:      "/usr/bin/bash",
		KeepProgramSpec: true,
	}
}
