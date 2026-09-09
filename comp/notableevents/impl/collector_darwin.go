// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableeventsimpl

import (
	"context"
	"errors"
	"sync"
	"time"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	darwinPollInterval       = 5 * time.Second
	darwinShutdownAckTimeout = time.Second
)

type notableEventsClient interface {
	GetEvents(context.Context) ([]notableeventtypes.Event, error)
	Ack(context.Context, []string) error
}

type sysProbeClient struct {
	client *sysprobeclient.CheckClient
}

type ackRequest struct {
	IDs []string `json:"ids"`
}

// GetEvents retrieves the current pending notable events from system-probe.
func (c *sysProbeClient) GetEvents(ctx context.Context) ([]notableeventtypes.Event, error) {
	return sysprobeclient.GetCheckWithContext[[]notableeventtypes.Event](ctx, c.client, sysconfig.NotableEventsModule)
}

// Ack tells system-probe which notable events the forwarding pipeline accepted.
func (c *sysProbeClient) Ack(ctx context.Context, ids []string) error {
	_, err := sysprobeclient.PostWithContext[struct{}](ctx, c.client, "/ack", ackRequest{IDs: ids}, sysconfig.NotableEventsModule)
	return err
}

type collector struct {
	outChan            chan<- eventPayload
	client             notableEventsClient
	pollInterval       time.Duration
	shutdownAckTimeout time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newPlatformCollector creates the macOS collector connected to the configured system-probe socket.
func newPlatformCollector(outChan chan<- eventPayload, cfg sysprobeconfig.Component) (*collector, error) {
	client := sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(cfg.GetString("system_probe_config.sysprobe_socket")),
	)
	return newCollectorWithClient(outChan, &sysProbeClient{client: client}, darwinPollInterval), nil
}

// newCollectorWithClient creates a poller with injectable transport and timing dependencies.
func newCollectorWithClient(outChan chan<- eventPayload, client notableEventsClient, pollInterval time.Duration) *collector {
	return &collector{
		outChan:            outChan,
		client:             client,
		pollInterval:       pollInterval,
		shutdownAckTimeout: darwinShutdownAckTimeout,
	}
}

// start launches the background system-probe polling loop once.
func (c *collector) start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.run(ctx)
	return nil
}

// stop cancels polling and waits for the background loop to exit.
func (c *collector) stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	c.wg.Wait()
}

// run polls immediately and then continues at the configured interval until cancellation.
func (c *collector) run(ctx context.Context) {
	defer c.wg.Done()

	c.poll(ctx)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.poll(ctx)
		}
	}
}

// poll forwards pending events and acknowledges only forwarding-pipeline acceptance.
func (c *collector) poll(ctx context.Context) {
	events, err := c.client.GetEvents(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, sysprobeclient.ErrNotStartedYet) {
			log.Debugf("Notable events: system-probe is not ready yet: %v", err)
		} else {
			log.Warnf("Notable events: failed to poll system-probe, will retry: %v", err)
		}
		return
	}

	successfulIDs := make([]string, 0, len(events))
processEvents:
	for _, event := range events {
		if ctx.Err() != nil {
			break
		}

		completion := make(chan error, 1)
		payload := eventPayload{
			Timestamp:  event.Timestamp,
			EventType:  event.EventType,
			Title:      event.Title,
			Message:    event.Message,
			Custom:     event.Custom,
			completion: completion,
		}
		select {
		case c.outChan <- payload:
		case <-ctx.Done():
			break processEvents
		}

		completed, canceled, submitErr := waitForSubmission(ctx, completion)
		if completed && submitErr == nil {
			successfulIDs = append(successfulIDs, event.ID)
		}
		if canceled {
			break processEvents
		}
	}

	if len(successfulIDs) == 0 {
		return
	}
	if ctx.Err() != nil {
		if err := c.ackAcceptedOnShutdown(successfulIDs); err != nil {
			log.Warnf("Notable events: failed final acknowledgement during shutdown: %v", err)
		}
		return
	}

	if err := c.client.Ack(ctx, successfulIDs); err != nil {
		if ctx.Err() != nil {
			// The lifecycle-bound request may have reached system-probe before
			// cancellation. ACK is idempotent, so retry the same batch exactly
			// once under a short independent shutdown deadline.
			err = c.ackAcceptedOnShutdown(successfulIDs)
		}
		if err == nil {
			return
		}
		log.Warnf("Notable events: failed to acknowledge delivered events, will retry: %v", err)
	}
}

// waitForSubmission preserves a completion that is already observable when
// cancellation wins while leaving later completions eligible for redelivery.
func waitForSubmission(ctx context.Context, completion <-chan error) (completed, canceled bool, submitErr error) {
	select {
	case submitErr = <-completion:
		return true, ctx.Err() != nil, submitErr
	case <-ctx.Done():
		select {
		case submitErr = <-completion:
			return true, true, submitErr
		default:
			return false, true, nil
		}
	}
}

func (c *collector) ackAcceptedOnShutdown(ids []string) error {
	timeout := c.shutdownAckTimeout
	if timeout <= 0 {
		timeout = darwinShutdownAckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.client.Ack(ctx, ids)
}
