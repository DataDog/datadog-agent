// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
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
				if !probe.constantOffsets.IsPresent(constantfetch.OffsetNameBPFMapStructName) {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}

			case "bpf.prog.name":
				if !probe.constantOffsets.IsPresent(constantfetch.OffsetNameBPFProgAuxStructName) {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}
			case "packet.filter":
				if probe.isRawPacketNotSupported() {
					return fmt.Errorf("%s is not available on this kernel version", field)
				}

				filter := rawpacket.Filter{
					BPFFilter: value.Value.(string),
					Policy:    rawpacket.PolicyAllow,
				}

				if _, err := rawpacket.FilterToInsts(0, filter, rawpacket.DefaultProgOpts()); err != nil {
					return err
				}
			}

			return nil
		},
		ExtraValidateRule: func(rule *eval.Rule) error {
			eventType, err := rule.GetEventType()
			if err != nil {
				return fmt.Errorf("unable to detect event type: %w", err)
			}

			switch eventType {
			case model.RawPacketFilterEventType.String():
				// the filter field is mandatory
				if len(rule.GetFieldValues("packet.filter")) == 0 {
					return errors.New("rules for the `packet` event type must use `packet.filter`")
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
