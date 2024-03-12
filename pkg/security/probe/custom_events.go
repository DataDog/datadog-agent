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
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// EventLostRead is the event used to report lost events detected from user space
// easyjson:json
type EventLostRead struct {
	events.CustomEventCommonFields
	Name string  `json:"map"`
	Lost float64 `json:"lost"`
}

// ToJSON marshal using json format
func (e EventLostRead) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost float64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostRead{
		Name: mapName,
		Lost: lost,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID, events.LostEventsRuleDesc), events.NewCustomEvent(model.CustomLostReadEventType, evt)
}

// EventLostWrite is the event used to report lost events detected from kernel space
// easyjson:json
type EventLostWrite struct {
	events.CustomEventCommonFields
	Name string            `json:"map"`
	Lost map[string]uint64 `json:"per_event"`
}

// ToJSON marshal using json format
func (e EventLostWrite) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]uint64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostWrite{
		Name: mapName,
		Lost: perEventPerCPU,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID, events.LostEventsRuleDesc), events.NewCustomEvent(model.CustomLostWriteEventType, evt)
}

func errorToEventType(err error) model.EventType {
	switch err.(type) {
	case dentry.ErrTruncatedParents, dentry.ErrTruncatedParentsERPC:
		return model.CustomTruncatedParentsEventType
	default:
		return model.UnknownEventType
	}
}

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
func NewAbnormalEvent(id string, description string, event *model.Event, err error) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() events.EventMarshaler {
		evt := AbnormalEvent{
			Event: serializers.NewEventSerializer(event, nil),
			Error: err.Error(),
		}
		evt.FillCustomEventCommonFields()
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.ResolveEventTime()

		return evt
	}

	return events.NewCustomRule(id, description), events.NewCustomEventLazy(errorToEventType(err), marshalerCtor)
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
	} `json:"container,omitempty"`
	EntrypointArgs []string `json:"args,omitempty"`
}

// ToJSON marshal using json format
func (e EBPFLessHelloMsgEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// NewEBPFLessHelloMsgEvent returns a eBPFLess hello custom event
func NewEBPFLessHelloMsgEvent(msg *ebpfless.HelloMsg, scrubber *procutil.DataScrubber) (*rules.Rule, *events.CustomEvent) {
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

	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.EBPFLessHelloMessageRuleID, events.EBPFLessHelloMessageRuleDesc), events.NewCustomEvent(model.UnknownEventType, evt)
}
