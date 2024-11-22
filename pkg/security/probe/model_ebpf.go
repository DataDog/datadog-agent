// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NewEBPFModel returns a new model with some extra field validation
func NewEBPFModel(probe *EBPFProbe) *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, _ eval.FieldValue) error {
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

func newEBPFEvent(fh *EBPFFieldHandlers) *model.Event {
	event := model.NewFakeEvent()
	event.FieldHandlers = fh
	return event
}

// newEBPFEventFromPCE returns a new event from a process cache entry
func newEBPFEventFromPCE(entry *model.ProcessCacheEntry, fh *EBPFFieldHandlers) *model.Event {
	eventType := model.ExecEventType
	if !entry.IsExec {
		eventType = model.ForkEventType
	}

	event := newEBPFEvent(fh)
	event.Type = uint32(eventType)
	event.TimestampRaw = uint64(time.Now().UnixNano())
	event.ProcessCacheEntry = entry
	event.ProcessContext = &entry.ProcessContext
	event.Exec.Process = &entry.Process
	event.ProcessContext.Process.ContainerID = entry.ContainerID
	event.ProcessContext.Process.CGroup = entry.CGroup

	return event
}
