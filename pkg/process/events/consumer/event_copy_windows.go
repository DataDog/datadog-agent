// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

package consumer

import (
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var _ = intern.Value{}

func (p *ProcessConsumer) Copy(event *smodel.Event) any {
	var result model.ProcessEvent

	valueEMEventType := uint32(event.GetEventType())
	result.EMEventType = valueEMEventType

	valueCollectionTime := event.GetTimestamp()
	result.CollectionTime = valueCollectionTime

	valueContainerID := event.GetContainerId()
	result.ContainerID = valueContainerID

	valuePpid := event.GetProcessPpid()
	result.Ppid = valuePpid

	if event.GetEventType() == smodel.ExecEventType {
		valueExecTime := event.GetProcessExecTime()
		result.ExecTime = valueExecTime
	}

	if event.GetEventType() == smodel.ExitEventType {
		valueExitTime := event.GetProcessExitTime()
		result.ExitTime = valueExitTime
	}

	if event.GetEventType() == smodel.ExitEventType {
		valueExitCode := event.GetExitCode()
		result.ExitCode = valueExitCode
	}
	return &result
}
