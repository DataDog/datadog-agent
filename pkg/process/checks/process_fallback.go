// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// useWLMCollection checks the configuration to use the workloadmeta process collector or not in linux
// TODO: process_config.process_collection.use_wlm is a temporary configuration for refactoring purposes
func (p *ProcessCheck) useWLMCollection() bool {
	log.Info("process_config.process_collection.use_wlm is not supported for non-linux platforms")
	return false
}

// processesByPID returns the processes by pid from the process probe for non-linux platforms
// because the workload meta process collection is only available on linux
func (p *ProcessCheck) processesByPID() (map[int32]*procutil.Process, error) {
	procs, err := p.probe.ProcessesByPID(p.clock.Now(), true)
	if err != nil {
		return nil, err
	}
	return procs, nil
}

// formatPorts is a stub for non-linux platforms
func formatPorts(_ []uint16) *model.PortInfo {
	return nil
}

// formatLanguage is a stub for non-linux platforms
func formatLanguage(_ *languagemodels.Language) model.Language {
	return model.Language_LANGUAGE_UNKNOWN
}

// formatServiceDiscovery is a stub for non-linux platforms
func formatServiceDiscovery(_ *procutil.Service) *model.ServiceDiscovery {
	return nil
}
