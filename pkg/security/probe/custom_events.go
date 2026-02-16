//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"encoding/base64"

	coretags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
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

// FailedDNSEvent is used to signal that a DNS packet failed to be decoded
// easyjson:json
type FailedDNSEvent struct {
	events.CustomEventCommonFields
	Payload string `json:"payload"`
}

// ToJSON marshal using json format
func (e FailedDNSEvent) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(e)
}

// NewAbnormalEvent returns the rule and a populated custom event for an abnormal event
func NewAbnormalEvent(acc *events.AgentContainerContext, id string, description string, event *model.Event, scrubber *utils.Scrubber, err error, opts *eval.Opts) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() events.EventMarshaler {
		evt := AbnormalEvent{
			Event: serializers.NewEventSerializer(event, nil, scrubber),
			Error: err.Error(),
		}
		evt.FillCustomEventCommonFields(acc)
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.ResolveEventTime()

		return evt
	}

	return events.NewCustomRule(id, description, opts), events.NewCustomEventLazy(model.CustomEventType, marshalerCtor)
}

// NewFailedDNSEvent returns the rule and a populated custom event for a failed dns packet decoding
func NewFailedDNSEvent(acc *events.AgentContainerContext, id string, description string, event *model.Event, opts *eval.Opts) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() events.EventMarshaler {
		evt := FailedDNSEvent{
			Payload: base64.StdEncoding.EncodeToString(event.FailedDNS.Payload),
		}
		evt.FillCustomEventCommonFields(acc)
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.ResolveEventTime()

		return evt
	}

	return events.NewCustomRule(id, description, opts), events.NewCustomEventLazy(model.CustomEventType, marshalerCtor)
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
func NewEBPFLessHelloMsgEvent(acc *events.AgentContainerContext, msg *ebpfless.HelloMsg, scrubber *utils.Scrubber, tagger tags.Tagger) (*rules.Rule, *events.CustomEvent) {
	args := msg.EntrypointArgs
	if scrubber != nil {
		args = scrubber.ScrubCommand(msg.EntrypointArgs)
	}

	evt := EBPFLessHelloMsgEvent{
		NSID: msg.NSID,
	}
	evt.Container.ID = string(msg.ContainerContext.ID)

	if tagger != nil {
		tags, err := tags.GetTagsOfContainer(tagger, containerutils.ContainerID(msg.ContainerContext.ID))
		if err != nil {
			seclog.Errorf("Failed to get tags for container %s: %v", msg.ContainerContext.ID, err)
		} else {
			evt.Container.Name = utils.GetTagValue(coretags.EcsContainerName, tags)
			evt.Container.ImageShortName = utils.GetTagValue(coretags.ShortImage, tags)
			evt.Container.ImageTag = utils.GetTagValue(coretags.ImageTag, tags)
		}
	}

	evt.EntrypointArgs = args

	evt.FillCustomEventCommonFields(acc)

	return events.NewCustomRule(events.EBPFLessHelloMessageRuleID, events.EBPFLessHelloMessageRuleDesc, nil), events.NewCustomEvent(model.CustomEventType, evt)
}
