// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	uatomic "go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	hostname      string
	reporter      event.Reporter
	client        *RuntimeSecurityClient
	running       uatomic.Bool
	wg            sync.WaitGroup
	connected     uatomic.Bool
	eventReceived uint64
	telemetry     *telemetry
	endpoints     *config.Endpoints
	cancel        context.CancelFunc
}

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(hostname string, reporter event.Reporter, endpoints *config.Endpoints) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, errors.Errorf("failed to initialize the telemetry reporter")
	}

	return &RuntimeSecurityAgent{
		client:    client,
		reporter:  reporter,
		hostname:  hostname,
		telemetry: telemetry,
		endpoints: endpoints,
	}, nil
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	rsa.cancel = cancel

	// Start the system-probe events listener
	go rsa.StartEventListener()
	// Send Runtime Security Agent telemetry
	go rsa.telemetry.run(ctx)
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.cancel()
	rsa.running.Store(false)
	rsa.wg.Wait()
	rsa.client.Close()
}

// StartEventListener starts listening for new events from system-probe
func (rsa *RuntimeSecurityAgent) StartEventListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	rsa.connected.Store(false)

	logTicker := newLogBackoffTicker()

	rsa.running.Store(true)
	for rsa.running.Load() == true {
		stream, err := rsa.client.GetEvents()
		if err != nil {
			rsa.connected.Store(false)

			select {
			case <-logTicker.C:
				msg := fmt.Sprintf("error while connecting to the runtime security module: %v", err)

				if e, ok := status.FromError(err); ok {
					switch e.Code() {
					case codes.Unavailable:
						msg += ", please check that the runtime security module is enabled in the system-probe.yaml config file"
					}
				}
				log.Error(msg)
			default:
				// do nothing
			}

			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		if !rsa.connected.Load() {
			rsa.connected.Store(true)

			log.Info("Successfully connected to the runtime security module")
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}
			log.Tracef("Got message from rule `%s` for event `%s`", in.RuleID, string(in.Data))

			atomic.AddUint64(&rsa.eventReceived, 1)

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// DispatchEvent dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	// For now simply log to Datadog
	rsa.reporter.ReportRaw(evt.GetData(), evt.Service, evt.GetTags()...)
}

// GetStatus returns the current status on the agent
func (rsa *RuntimeSecurityAgent) GetStatus() map[string]interface{} {
	base := map[string]interface{}{
		"connected":     rsa.connected.Load(),
		"eventReceived": atomic.LoadUint64(&rsa.eventReceived),
		"endpoints":     rsa.endpoints.GetStatus(),
	}

	if rsa.client != nil {
		cfStatus, err := rsa.client.GetStatus()
		if err == nil {
			if cfStatus.Environment != nil {
				environment := map[string]interface{}{
					"warnings":       cfStatus.Environment.Warnings,
					"kernelLockdown": cfStatus.Environment.KernelLockdown,
				}
				if cfStatus.Environment.Constants != nil {
					environment["constantFetchers"] = cfStatus.Environment.Constants
				}
				base["environment"] = environment
			}
			if cfStatus.SelfTests != nil {
				selfTests := map[string]interface{}{
					"LastTimestamp": cfStatus.SelfTests.LastTimestamp,
					"Success":       cfStatus.SelfTests.Success,
					"Fails":         cfStatus.SelfTests.Fails,
				}
				base["selfTests"] = selfTests
			}
		}
	}

	return base
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connect error logs
func newLogBackoffTicker() *backoff.Ticker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.MaxElapsedTime = 0
	expBackoff.Reset()
	return backoff.NewTicker(expBackoff)
}
