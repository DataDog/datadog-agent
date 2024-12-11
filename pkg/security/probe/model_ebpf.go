// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// NewEBPFModel returns a new model with some extra field validation
func NewEBPFModel(probe *EBPFProbe) *model.Model {
	return &model.Model{
		ExtraValidateFieldFnc: func(field eval.Field, value eval.FieldValue) error {
			switch field {
			case "bpf.map.name":
				if offset, found := probe.constantOffsets[constantfetch.OffsetNameBPFMapStructName]; !found || offset == constantfetch.ErrorSentinel {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}

			case "bpf.prog.name":
				if offset, found := probe.constantOffsets[constantfetch.OffsetNameBPFProgAuxStructName]; !found || offset == constantfetch.ErrorSentinel {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}
			case "packet.filter":
				if probe.isRawPacketNotSupported() {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}
				if _, err := rawpacket.BPFFilterToInsts(0, value.Value.(string), rawpacket.DefaultProgOpts); err != nil {
					return err
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
