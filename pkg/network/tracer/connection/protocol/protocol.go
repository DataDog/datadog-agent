// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package protocol

import (
	"fmt"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	manager "github.com/DataDog/ebpf-manager"
)

var (
	protocolClassifierSocketFilterFuncName      = "socket__classifier"
	protocolClassifierEntrySocketFilterFuncName = "socket__classifier_entry"

	tailCalls = []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           0,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  probes.ProtocolClassifierSocketFilter,
				EBPFFuncName: protocolClassifierSocketFilterFuncName,
				UID:          probes.UID,
			},
		},
	}
)

func EnableProtocolClassification(c *config.Config, m *manager.Manager, mgrOptions *manager.Options) (closeProtocolClassifierSocketFilterFn func(), undefinedProbes []manager.ProbeIdentificationPair, err error) {
	if c.ClassificationSupported() && c.CollectTCPConns {
		socketFilterProbe, _ := m.GetProbe(manager.ProbeIdentificationPair{
			EBPFSection:  probes.ProtocolClassifierEntrySocketFilter,
			EBPFFuncName: protocolClassifierEntrySocketFilterFuncName,
			UID:          probes.UID,
		})
		if socketFilterProbe == nil {
			return nil, nil, fmt.Errorf("error retrieving protocol classifier socket filter")
		}

		closeProtocolClassifierSocketFilterFn, err = filter.HeadlessSocketFilter(c, socketFilterProbe)
		if err != nil {
			return nil, nil, fmt.Errorf("error enabling protocol classifier: %s", err)
		}

		undefinedProbes = append(undefinedProbes, tailCalls[0].ProbeIdentificationPair)
		mgrOptions.TailCallRouter = append(mgrOptions.TailCallRouter, tailCalls...)
	} else {
		// Kernels < 4.7.0 do not know about the per-cpu array map used
		// in classification, preventing the program to load even though
		// we won't use it. We change the type to a simple array map to
		// circumvent that.
		mgrOptions.MapSpecEditors[string(probes.ProtocolClassificationBufMap)] = manager.MapSpecEditor{
			Type:       ebpf.Array,
			EditorFlag: manager.EditType,
		}
	}

	return closeProtocolClassifierSocketFilterFn, undefinedProbes, err
}
