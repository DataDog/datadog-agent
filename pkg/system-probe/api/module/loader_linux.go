// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func isEBPFRequired(modules []types.SystemProbeModuleComponent) bool {
	for _, m := range modules {
		if m.NeedsEBPF() {
			return true
		}
	}
	return false
}

func isEBPFOptional(modules []types.SystemProbeModuleComponent) bool {
	for _, m := range modules {
		if m.OptionalEBPF() {
			return true
		}
	}
	return false
}

func preRegister(_ *sysconfigtypes.Config, rcclient rcclient.Component, modules []types.SystemProbeModuleComponent) error {
	needed := isEBPFRequired(modules)
	if needed || isEBPFOptional(modules) {
		err := ebpf.Setup(ebpf.NewConfig(), rcclient)
		if err != nil && !needed {
			log.Warnf("ignoring eBPF setup error: %v", err)
			return nil
		}

		return err
	}
	return nil
}

func postRegister(cfg *sysconfigtypes.Config, modules []types.SystemProbeModuleComponent) error {
	needBTFFlush := isEBPFRequired(modules) || isEBPFOptional(modules)

	if cfg.TelemetryEnabled && ebpf.ContentionCollector != nil {
		needBTFFlush = true
		if err := ebpf.ContentionCollector.Initialize(ebpf.TrackAllEBPFResources); err != nil {
			// do not prevent system-probe from starting if lock contention collector fails
			log.Errorf("failed to initialize ebpf lock contention collector: %v", err)
		}
	}

	if needBTFFlush {
		ebpf.FlushBTF()
	}
	return nil
}
