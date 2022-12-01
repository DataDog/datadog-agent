// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package uprobe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
)

const UprobeUID = "vuln_detector"
const UprobeSection = "uprobe/vuln_detector"
const UprobeFuncName = "uprobe_vuln_detector"
const UprobeConstantName = "vuln_id"

func attachProbe(m *manager.Manager, u *uprobe) error {
	pID := manager.ProbeIdentificationPair{
		UID:          fmt.Sprintf("vuln_detector_%d", u.id),
		EBPFSection:  UprobeSection,
		EBPFFuncName: UprobeFuncName,
	}
	p := &manager.Probe{
		ProbeIdentificationPair: pID,
		BinaryPath:              u.desc.Path,
		HookFuncName:            u.desc.FunctionName,
		CopyProgram:             true,
		Enabled:                 true,
	}
	ce := manager.ConstantEditor{
		Name:                     UprobeConstantName,
		Value:                    uint64(u.id),
		ProbeIdentificationPairs: []manager.ProbeIdentificationPair{pID},
	}
	err := m.CloneProgram(UprobeUID, p, []manager.ConstantEditor{ce}, nil)
	if err != nil {
		return err
	}

	u.pID = pID

	seclog.Infof("attached %s:%s with UID %s and vuln_id %d\n", u.desc.Path, u.desc.FunctionName, u.pID.UID, u.id)
	return nil
}
