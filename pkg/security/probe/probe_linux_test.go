// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type MockEventHandler struct{}

func (MockEventHandler) HandleEvent(incomingEvent interface{}) {
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

func (MockEventHandler) Copy(incomingEvent *model.Event) interface{} {
	return incomingEvent
}

func BenchmarkSendSpecificEvent(b *testing.B) {
	eventHandler := MockEventHandler{}

	type fields struct {
		Opts                 Opts
		StatsdClient         statsd.ClientInterface
		startTime            time.Time
		ctx                  context.Context
		cancelFnc            context.CancelFunc
		scrubber             *procutil.DataScrubber
		eventHandlers        [model.MaxAllEventType][]EventHandler
		customEventHandlers  [model.MaxAllEventType][]CustomEventHandler
		discarderRateLimiter *rate.Limiter
		resolvers            *resolvers.Resolvers
		fieldHandlers        *FieldHandlers
		event                *model.Event
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
			//fields: fields{eventHandlers: [model.MaxAllEventType][]EventHandler{}},
			args: args{event: model.NewDefaultEvent()},
		},
	}

	for _, tt := range tests {
		p := &Probe{
			Opts:                 tt.fields.Opts,
			StatsdClient:         tt.fields.StatsdClient,
			startTime:            tt.fields.startTime,
			ctx:                  tt.fields.ctx,
			cancelFnc:            tt.fields.cancelFnc,
			scrubber:             tt.fields.scrubber,
			eventHandlers:        tt.fields.eventHandlers,
			customEventHandlers:  tt.fields.customEventHandlers,
			discarderRateLimiter: tt.fields.discarderRateLimiter,
			resolvers:            tt.fields.resolvers,
			fieldHandlers:        tt.fields.fieldHandlers,
			event:                tt.fields.event,
		}

		for i := 0; i < 10; i++ {
			p.AddEventHandler(model.ExecEventType, eventHandler)
		}

		for i := 0; i < b.N; i++ {
			p.sendSpecificEvent(tt.args.event)
		}
	}
}
