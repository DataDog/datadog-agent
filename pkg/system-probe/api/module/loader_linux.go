// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func isEBPFRequired(factories []Factory) bool {
	for _, f := range factories {
		if f.NeedsEBPF() {
			return true
		}
	}
	return false
}

func isEBPFOptional(factories []Factory) bool {
	for _, f := range factories {
		if f.OptionalEBPF {
			return true
		}
	}
	return false
}

func preRegister(_ *sysconfigtypes.Config, moduleFactories []Factory) error {
	needed := isEBPFRequired(moduleFactories)
	if needed || isEBPFOptional(moduleFactories) {
		err := ebpf.Setup(ebpf.NewConfig())
		if err != nil && !needed {
			log.Warnf("ignoring eBPF setup error: %v", err)
			return nil
		}

		return err
	}
	return nil
}

func postRegister(cfg *sysconfigtypes.Config, moduleFactories []Factory) error {
	if isEBPFRequired(moduleFactories) || isEBPFOptional(moduleFactories) {
		ebpf.FlushBTF()
	}
	if cfg.TelemetryEnabled && ebpf.ContentionCollector != nil {
		if err := ebpf.ContentionCollector.Initialize(ebpf.TrackAllEBPFResources); err != nil {
			// do not prevent system-probe from starting if lock contention collector fails
			log.Errorf("failed to initialize ebpf lock contention collector: %v", err)
		}
	}
	return nil
}
