// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func isEBPFRequired(factories []*Factory) bool {
	for _, f := range factories {
		if f.NeedsEBPF() {
			return true
		}
	}
	return false
}

func isEBPFOptional(factories []*Factory) bool {
	for _, f := range factories {
		if f.OptionalEBPF {
			return true
		}
	}
	return false
}

func preRegister(cfg *sysconfigtypes.Config, rcclient rcclient.Component, telemetry telemetry.Component, moduleFactories []*Factory) error {
	needed := isEBPFRequired(moduleFactories)
	contentionWillLoad := cfg.TelemetryEnabled && ebpf.ContentionCollector != nil
	if needed || isEBPFOptional(moduleFactories) || contentionWillLoad {
		err := ebpf.Setup(ebpf.NewConfig(), rcclient, telemetry)
		if err != nil && !needed {
			log.Warnf("ignoring eBPF setup error: %v", err)
			return nil
		}

		return err
	}
	return nil
}

func postRegister(cfg *sysconfigtypes.Config, moduleFactories []*Factory) error {
	needBTFFlush := isEBPFRequired(moduleFactories) || isEBPFOptional(moduleFactories)

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
