// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	hostname             string
	reporter             common.RawReporter
	client               *RuntimeSecurityClient
	running              *atomic.Bool
	wg                   sync.WaitGroup
	connected            *atomic.Bool
	eventReceived        *atomic.Uint64
	activityDumpReceived *atomic.Uint64
	telemetry            *telemetry
	endpoints            *config.Endpoints
	cancel               context.CancelFunc

	// activity dump
	storage *dump.ActivityDumpStorageManager
}

// RSAOptions represents the runtime security agent options
type RSAOptions struct {
	LogProfiledWorkloads    bool
	IgnoreDDAgentContainers bool
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
	// Start activity dumps listener
	go rsa.StartActivityDumpListener()

	if rsa.telemetry != nil {
		// Send Runtime Security Agent telemetry
		go rsa.telemetry.run(ctx, rsa)
	}
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
			log.Tracef("Got activity dump [%s]", msg.GetDump().GetMetadata().GetName())

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
	rsa.reporter.ReportRaw(evt.GetData(), evt.Service, evt.GetTags()...)
}

// DispatchActivityDump forwards an activity dump message to the backend
func (rsa *RuntimeSecurityAgent) DispatchActivityDump(msg *api.ActivityDumpStreamMessage) {
	// parse dump from message
	dump, err := dump.NewActivityDumpFromMessage(msg.GetDump())
	if err != nil {
		log.Errorf("%v", err)
		return
	}
	if rsa.telemetry != nil {
		// register for telemetry for this container
		imageName, imageTag := dump.GetImageNameTag()
		rsa.telemetry.registerProfiledContainer(imageName, imageTag)

		raw := bytes.NewBuffer(msg.GetData())

		for _, requests := range dump.StorageRequests {
			if err := rsa.storage.PersistRaw(requests, dump, raw); err != nil {
				log.Errorf("%v", err)
			}
		}
	}
}

// GetStatus returns the current status on the agent
func (rsa *RuntimeSecurityAgent) GetStatus() map[string]interface{} {
	base := map[string]interface{}{
		"connected":            rsa.connected.Load(),
		"eventReceived":        rsa.eventReceived.Load(),
		"activityDumpReceived": rsa.activityDumpReceived.Load(),
	}

	if rsa.endpoints != nil {
		base["endpoints"] = rsa.endpoints.GetStatus()
	}

	if rsa.client != nil {
		cfStatus, err := rsa.client.GetStatus()
		if err == nil {
			if cfStatus.Environment != nil {
				environment := map[string]interface{}{
					"warnings":       cfStatus.Environment.Warnings,
					"kernelLockdown": cfStatus.Environment.KernelLockdown,
					"mmapableMaps":   cfStatus.Environment.UseMmapableMaps,
					"ringBuffer":     cfStatus.Environment.UseRingBuffer,
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
