// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

package examples

import (
	"go4.org/intern"

	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var _ = intern.Value{}

func (fc *SimpleEventConsumer) Copy(event *smodel.Event) any {
	var result SimpleEvent

	valueType := uint32(event.GetEventType())
	result.Type = valueType

	if event.GetEventType() == smodel.ExecEventType {
		valueExecFilePath := event.GetExecFilePath()
		result.ExecFilePath = valueExecFilePath
	}

	if event.GetEventType() == smodel.ExecEventType {
		valueEnvp := event.GetProcessEnvp()
		result.Envp = valueEnvp
	}
	return &result
}
