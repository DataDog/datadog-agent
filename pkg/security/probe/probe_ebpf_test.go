// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type MockEventHandler struct{}

func (MockEventHandler) HandleEvent(incomingEvent any) {
	event, ok := incomingEvent.(*model.Event)
	if !ok {
		seclog.Errorf("Event is not a security model event")
		return
	}

	// event already marked with an error, skip it
	if event.Error != nil {
		return
	}

	// if the event should have been discarded in kernel space, we don't need to evaluate it
	if event.IsSavedByActivityDumps() {
		return
	}
}

func (MockEventHandler) Copy(incomingEvent *model.Event) any {
	return incomingEvent
}

// go test github.com/DataDog/datadog-agent/pkg/security/probe -v -bench="BenchmarkSendSpecificEvent" -run=^# -benchtime=10s -count=7 | tee new.txt
// benchstat old.txt new.txt
func BenchmarkSendSpecificEvent(b *testing.B) {
	eventHandler := MockEventHandler{}
	execEvent := model.NewFakeEvent()
	execEvent.Type = uint32(model.ExecEventType)

	type fields struct {
		eventHandlers       [model.MaxAllEventType][]EventHandler
		customEventHandlers [model.MaxAllEventType][]CustomEventHandler
	}
	type args struct {
		event *model.Event
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "basic case",
			args: args{event: execEvent},
		},
	}

	for _, tt := range tests {
		p := &Probe{
			eventHandlers:       tt.fields.eventHandlers,
			customEventHandlers: tt.fields.customEventHandlers,
		}

		for i := 0; i < 10; i++ {
			p.AddEventHandler(model.ExecEventType, eventHandler)
		}

		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p.sendEventToSpecificEventTypeHandlers(tt.args.event)
			}
		})
	}
}
