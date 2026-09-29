// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventplatformimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	eventplatformreceiver "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// IsolatedForwarder uses the ordinary software and NDM event-platform pipelines
// with explicit configuration, recording transport and completion accounting.
// It is constructed only by the simulator. Production construction is unchanged.
type IsolatedForwarder struct {
	inner   *defaultEventPlatformForwarder
	Tracker *transaction.DeliveryTracker
}

func NewIsolatedForwarder(cfg config.Component, host hostname.Component, compressor compression.Component, secrets secrets.Component, transport http.RoundTripper) (*IsolatedForwarder, error) {
	f := &IsolatedForwarder{Tracker: transaction.NewDeliveryTracker()}
	dc := client.NewDestinationsContext()
	dc.Transport = transport
	dc.OnDelivery = f.completed
	dc.Start()
	f.inner = &defaultEventPlatformForwarder{pipelines: map[string]*passthroughPipeline{}, destinationsCtx: dc}
	receiver := eventplatformreceiver.NewReceiver(host, cfg).Comp
	for i, desc := range []passthroughPipelineDesc{softwareInventoryPipeline(), getNDMCorePipelines()[0]} {
		pipeline, err := newHTTPPassthroughPipeline(cfg, receiver, compressor, desc, dc, i, host.GetSafe(context.Background()), secrets)
		if err != nil {
			dc.Stop()
			return nil, fmt.Errorf("construct isolated %s pipeline: %w", desc.eventType, err)
		}
		f.inner.pipelines[desc.eventType] = pipeline
	}
	return f, nil
}

func (f *IsolatedForwarder) Start() { f.inner.Start() }
func (f *IsolatedForwarder) Stop()  { f.inner.destinationsCtx.Stop(); f.inner.Stop() }

// Send blocks for queue capacity, while respecting cancellation. Completion
// accounting includes every configured HTTP destination, after its retries.
func (f *IsolatedForwarder) Send(ctx context.Context, msg *message.Message, eventType string) error {
	p, ok := f.inner.pipelines[eventType]
	if !ok {
		return fmt.Errorf("unsupported isolated event type %s", eventType)
	}
	done := f.Tracker.Register()
	var mu sync.Mutex
	remaining := p.deliveryDestinations
	completed := false
	msg.DeliveryCallback = func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if completed {
			return
		}
		remaining--
		if err != nil {
			completed = true
			done(errors.New("event-platform delivery failed"))
		} else if remaining == 0 {
			completed = true
			done(nil)
		}
	}
	select {
	case p.in <- msg:
		return nil
	case <-ctx.Done():
		done(ctx.Err())
		return ctx.Err()
	}
}

func (f *IsolatedForwarder) completed(payload *message.Payload, _ string, err error) {
	for _, meta := range payload.MessageMetas {
		if meta.DeliveryCallback != nil {
			meta.DeliveryCallback(err)
		}
	}
}
