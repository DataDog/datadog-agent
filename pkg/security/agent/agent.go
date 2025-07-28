// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	statsdClient            statsd.ClientInterface
	hostname                string
	reporter                common.RawReporter
	client                  *RuntimeSecurityClient
	running                 *atomic.Bool
	wg                      sync.WaitGroup
	connected               *atomic.Bool
	eventReceived           *atomic.Uint64
	activityDumpReceived    *atomic.Uint64
	profContainersTelemetry *profContainersTelemetry
	endpoints               *config.Endpoints
	cancel                  context.CancelFunc

	// activity dump
	storage ADStorage
}

// ADStorage represents the interface for the activity dump storage
type ADStorage interface {
	backend.ActivityDumpHandler

	SendTelemetry(_ statsd.ClientInterface)
}

// RSAOptions represents the runtime security agent options
type RSAOptions struct {
	LogProfiledWorkloads bool
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start(reporter common.RawReporter, endpoints *config.Endpoints) {
	rsa.reporter = reporter
	rsa.endpoints = endpoints

	ctx, cancel := context.WithCancel(context.Background())
	rsa.cancel = cancel

	rsa.running.Store(true)
	// Start the system-probe events listener
	go rsa.StartEventListener()

	if runtime.GOOS == "linux" {
		// Start activity dumps listener
		go rsa.StartActivityDumpListener()
		go rsa.startActivityDumpStorageTelemetry(ctx)
	}

	if rsa.profContainersTelemetry != nil {
		// Send Profiled Containers telemetry
		go rsa.profContainersTelemetry.run(ctx)
	}
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.cancel()
	rsa.running.Store(false)
	rsa.client.Close()
	rsa.wg.Wait()
}

// StartEventListener starts listening for new events from system-probe
func (rsa *RuntimeSecurityAgent) StartEventListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	rsa.connected.Store(false)

	logTicker := newLogBackoffTicker()

	for rsa.running.Load() {
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
				seclog.Errorf("%s", msg)
			default:
				// do nothing
			}

			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		if !rsa.connected.Load() {
			rsa.connected.Store(true)

			seclog.Infof("Successfully connected to the runtime security module")
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}

			if seclog.DefaultLogger.IsTracing() {
				seclog.DefaultLogger.Tracef("Got message from rule `%s` for event `%s`", in.RuleID, string(in.Data))
			}

			rsa.eventReceived.Inc()

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// StartActivityDumpListener starts listening for new activity dumps from system-probe
func (rsa *RuntimeSecurityAgent) StartActivityDumpListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	for rsa.running.Load() {
		stream, err := rsa.client.GetActivityDumpStream()
		if err != nil {
			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			// Get new activity dump from stream
			msg, err := stream.Recv()
			if err == io.EOF || msg == nil {
				break
			}

			if seclog.DefaultLogger.IsTracing() {
				seclog.DefaultLogger.Tracef("Got activity dump [%s]", msg.GetSelector())
			}

			rsa.activityDumpReceived.Inc()

			// Dispatch activity dump
			rsa.DispatchActivityDump(msg)
		}
	}
}

// DispatchEvent dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	if rsa.reporter == nil {
		return
	}
	rsa.reporter.ReportRaw(evt.GetData(), evt.Service, evt.Timestamp.AsTime(), evt.GetTags()...)
}

// DispatchActivityDump forwards an activity dump message to the backend
func (rsa *RuntimeSecurityAgent) DispatchActivityDump(msg *api.ActivityDumpStreamMessage) {
	selector := msg.GetSelector()
	image := selector.GetName()
	tag := selector.GetTag()

	if rsa.profContainersTelemetry != nil {
		// register for telemetry for this container
		if image != "" {
			rsa.profContainersTelemetry.registerProfiledContainer(image, tag)
		}
	}

	// storage might be nil, on windows for example
	if rsa.storage != nil {
		err := rsa.storage.HandleActivityDump(image, tag, msg.GetHeader(), msg.GetData())
		if err != nil {
			seclog.Errorf("couldn't handle activity dump: %v", err)
		}
	}
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

func (rsa *RuntimeSecurityAgent) startActivityDumpStorageTelemetry(ctx context.Context) {
	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			if rsa.storage != nil {
				rsa.storage.SendTelemetry(rsa.statsdClient)
			}
		}
	}
}
