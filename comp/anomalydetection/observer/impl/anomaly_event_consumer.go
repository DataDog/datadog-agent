// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
)

// anomalyEventConsumer subscribes to the anomaly events the isolated anomaly
// detection process publishes on a FIT broadcast ring, and logs every event it
// receives.
//
// It is a passive consumer: it feeds nothing into the pipeline, and it never
// blocks the observer. The publisher owns the endpoint, so a missing publisher
// is expected (the process may start later or restart) and the subscription is
// retried instead of failing the observer's startup.
type anomalyEventConsumer struct {
	endpoint      string
	retryInterval time.Duration

	received atomic.Uint64
	cancel   context.CancelFunc
	done     chan struct{}
}

// newAnomalyEventConsumer creates a consumer that subscribes to the configured
// broadcast endpoint. It returns a nil consumer without an error when this
// platform has no FIT transport, so the observer keeps running.
func newAnomalyEventConsumer(cfg anomalydetectionconfig.AnomalyEventsConfig) (*anomalyEventConsumer, error) {
	if !checksfit.Supported {
		logging.Warnf("%s is enabled but FIT is unavailable on this platform; anomaly events are not received",
			anomalydetectionconfig.AnomalyEventsEnabledConfigKey)
		return nil, nil
	}
	endpoint, err := fitcore.ParseEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid anomaly events FIT endpoint %q: %w", cfg.Endpoint, err)
	}

	consumer := &anomalyEventConsumer{
		endpoint:      cfg.Endpoint,
		retryInterval: cfg.RetryInterval,
		done:          make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	consumer.cancel = cancel
	go consumer.run(ctx, fitcore.NewSubscriberConfig(endpoint))
	return consumer, nil
}

// run subscribes and receives until the context ends, rejoining after a failed
// or ended subscription.
//
// FIT carries no liveness signal, so a subscription parked on a silent ring
// never notices that its publisher is gone. Rejoining therefore happens when a
// subscription fails or ends, which covers a publisher that is not up yet and a
// session that broke; a publisher that stops without closing its session leaves
// this consumer parked until the observer stops.
func (c *anomalyEventConsumer) run(ctx context.Context, config fitcore.SubscriberConfig) {
	defer close(c.done)
	for {
		subscriber, err := checksfit.SubscribeEventSubscriberContext(ctx, config)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logging.Debugf("no anomaly event publisher at %s yet: %v; retrying in %s", c.endpoint, err, c.retryInterval)
			if !sleepUntil(ctx, c.retryInterval) {
				return
			}
			continue
		}

		logging.Infof("subscribed to anomaly events on %s (session %d, slot %d)",
			c.endpoint, subscriber.SessionID(), subscriber.SlotID())
		c.serve(ctx, subscriber)
		if err := subscriber.Unsubscribe(); err != nil {
			// The publisher may already be gone, which makes the leave fail.
			logging.Debugf("unsubscribing from anomaly events on %s failed: %v", c.endpoint, err)
		}
		if err := subscriber.Close(); err != nil {
			logging.Debugf("closing the anomaly event subscription on %s failed: %v", c.endpoint, err)
		}
		if ctx.Err() != nil {
			return
		}
		if !sleepUntil(ctx, c.retryInterval) {
			return
		}
	}
}

// serve logs events until the subscription ends or the context ends.
func (c *anomalyEventConsumer) serve(ctx context.Context, subscriber *checksfit.EventSubscriber) {
	for {
		event, err := subscriber.ReceiveContext(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, fitcore.ErrCancelled) {
				return
			}
			logging.Warnf("anomaly event subscription on %s ended: %v; resubscribing", c.endpoint, err)
			return
		}

		c.received.Add(1)
		// WARN by design: an anomaly is meant to be noticed in the agent log.
		logging.Warnf("Anomaly event received: %s (%s) at %d", event.Title, event.Description, event.Timestamp)
	}
}

// close stops the consumer and waits for its goroutine to finish.
func (c *anomalyEventConsumer) close() error {
	if c == nil {
		return nil
	}
	if c.cancel != nil {
		c.cancel()
		<-c.done
	}
	return nil
}

// stats returns how many events were logged. Used by tests.
func (c *anomalyEventConsumer) stats() uint64 {
	if c == nil {
		return 0
	}
	return c.received.Load()
}
