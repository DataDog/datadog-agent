//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// AbnormalEvent is used to report that a path resolution failed for a suspicious reason
// easyjson:json
type AbnormalEvent struct {
	events.CustomEventCommonFields
	Event *serializers.EventSerializer `json:"triggering_event"`
	Error string                       `json:"error"`
}

// ToJSON marshal using json format
func (a AbnormalEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(a)
}

// NewAbnormalEvent returns the rule and a populated custom event for a abnormal event
func NewAbnormalEvent(acc *events.AgentContainerContext, id string, description string, event *model.Event, err error) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() events.EventMarshaler {
		evt := AbnormalEvent{
			Event: serializers.NewEventSerializer(event, nil),
			Error: err.Error(),
		}
		evt.FillCustomEventCommonFields(acc)
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.ResolveEventTime()

		return evt
	}

	return events.NewCustomRule(id, description), events.NewCustomEventLazy(model.CustomEventType, marshalerCtor)
}

// EBPFLessHelloMsgEvent defines a hello message
// easyjson:json
type EBPFLessHelloMsgEvent struct {
	events.CustomEventCommonFields

	NSID      uint64 `json:"nsid,omitempty"`
	Container struct {
		ID             string `json:"id,omitempty"`
		Name           string `json:"name,omitempty"`
		ImageShortName string `json:"short_name,omitempty"`
		ImageTag       string `json:"image_tag,omitempty"`
	} `json:"workload_container,omitempty"`
	EntrypointArgs []string `json:"args,omitempty"`
}

// ToJSON marshal using json format
func (e EBPFLessHelloMsgEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// NewEBPFLessHelloMsgEvent returns a eBPFLess hello custom event
func NewEBPFLessHelloMsgEvent(acc *events.AgentContainerContext, msg *ebpfless.HelloMsg, scrubber *procutil.DataScrubber) (*rules.Rule, *events.CustomEvent) {
	args := msg.EntrypointArgs
	if scrubber != nil {
		args, _ = scrubber.ScrubCommand(msg.EntrypointArgs)
	}

	evt := EBPFLessHelloMsgEvent{
		NSID: msg.NSID,
	}
	evt.Container.ID = msg.ContainerContext.ID
	evt.Container.Name = msg.ContainerContext.Name
	evt.Container.ImageShortName = msg.ContainerContext.ImageShortName
	evt.Container.ImageTag = msg.ContainerContext.ImageTag
	evt.EntrypointArgs = args

	evt.FillCustomEventCommonFields(acc)

	return events.NewCustomRule(events.EBPFLessHelloMessageRuleID, events.EBPFLessHelloMessageRuleDesc), events.NewCustomEvent(model.CustomEventType, evt)
}
