// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package module

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventServer represents a gRPC server in charge of receiving events sent by
// the runtime security system-probe module and forwards them to Datadog
type EventServer struct {
	msgs chan *api.SecurityEventMessage
}

// GetEvents waits for security events
func (e *EventServer) GetEvents(params *api.GetParams, stream api.SecurityModule_GetEventsServer) error {
	msgs := 10
LOOP:
	for {
		select {
		case msg := <-e.msgs:
			if err := stream.Send(msg); err != nil {
				return err
			}
			msgs--
		case <-time.After(time.Second):
			break LOOP
		}

		if msgs <= 0 {
			break
		}
	}

	return nil
}

// SendEvent forwards events sent by the runtime security module to Datadog
func (e *EventServer) SendEvent(rule *eval.Rule, event eval.Event) {
	data, err := json.Marshal(rules.RuleEvent{Event: event, RuleID: rule.ID})
	if err != nil {
		return
	}
	tags := append(rule.Tags, "rule_id:"+rule.ID)
	tags = append(tags, event.(*sprobe.Event).GetTags()...)
	log.Infof("Sending event message for rule `%s` to security-agent `%s` with tags %v", rule.ID, string(data), tags)

	msg := &api.SecurityEventMessage{
		RuleID: rule.ID,
		Type:   event.GetType(),
		Tags:   tags,
		Data:   data,
	}

	select {
	case e.msgs <- msg:
		break
	default:
		// Do not wait for the channel to free up, we don't want to delay the processing pipeline further
		log.Warnf("the event server channel is full, an event of ID %v was dropped", msg.RuleID)
		break
	}
}

// NewEventServer returns a new gRPC event server
func NewEventServer() *EventServer {
	return &EventServer{
		msgs: make(chan *api.SecurityEventMessage, 5),
	}
}
