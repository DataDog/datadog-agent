// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
)

var eventZero model.Event

// NewModel returns a new model with some extra field validation
func NewModel(probe *Probe) *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, fieldValue eval.FieldValue) error {
			switch field {
			case "bpf.map.name":
				if offset, found := probe.constantOffsets[constantfetch.OffsetNameBPFMapStructName]; !found || offset == constantfetch.ErrorSentinel {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}

			case "bpf.prog.name":
				if offset, found := probe.constantOffsets[constantfetch.OffsetNameBPFProgAuxStructName]; !found || offset == constantfetch.ErrorSentinel {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}
			}

			return nil
		},
	}
}

// NewEvent returns a new event
func NewEvent(fh *FieldHandlers) *model.Event {
	return &model.Event{
		FieldHandlers: fh,
	}
}
