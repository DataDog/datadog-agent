// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type serviceExtractor struct {
	*parser.ServiceExtractor
}

func newServiceExtractor(sysprobeconfig sysprobeconfig.Component) *serviceExtractor {
	serviceExtractorEnabled := sysprobeconfig.GetBool("system_probe_config.process_service_inference.enabled")
	useWindowsServiceName := sysprobeconfig.GetBool("system_probe_config.process_service_inference.use_windows_service_name")
	useImprovedAlgorithm := sysprobeconfig.GetBool("system_probe_config.process_service_inference.use_improved_algorithm")

	return &serviceExtractor{
		ServiceExtractor: parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm),
	}
}

func (s *serviceExtractor) process(event *process) {
	if event.EventType != model.ExecEventType && event.EventType != model.ForkEventType {
		return
	}

	s.ExtractSingle(&procutil.Process{
		Pid:     int32(event.Pid),
		Cmdline: event.Cmdline,
		Cwd:     event.Cwd,
	})
}

func (s *serviceExtractor) handleDeadProcess(pid uint32) {
	s.Remove(int32(pid))
}
