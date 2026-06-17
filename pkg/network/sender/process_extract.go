// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type processNameExtractor struct {
	*parser.ProcessNameExtractor
}

func newProcessNameExtractor() *processNameExtractor {
	return &processNameExtractor{
		ProcessNameExtractor: parser.NewProcessNameExtractor(),
	}
}

func (s *processNameExtractor) process(event *process) {
	if event.EventType != model.ExecEventType && event.EventType != model.ForkEventType {
		return
	}
	s.ExtractSingle(&procutil.Process{
		Pid:  int32(event.Pid),
		Comm: event.Comm,
		Exe:  event.Exe,
	})
}

func (s *processNameExtractor) handleDeadProcess(pid uint32) {
	s.Remove(int32(pid))
}
