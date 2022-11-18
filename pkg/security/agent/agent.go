// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/atomic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
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
	sbomReceived         *atomic.Uint64
	telemetry            *telemetry
	endpoints            *config.Endpoints
	cancel               context.CancelFunc

	// activity dump
	storage *probe.ActivityDumpStorageManager

	// sbom
	sbomRemoteStorage *probe.SBOMRemoteStorage
}

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(hostname string) (*RuntimeSecurityAgent, error) {
	client, err := NewRuntimeSecurityClient()
	if err != nil {
		return nil, err
	}

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, errors.New("failed to initialize the telemetry reporter")
	}

	storage, err := probe.NewSecurityAgentStorageManager()
	if err != nil {
		return nil, err
	}

	sbomRemoteStorage, err := probe.NewSBOMRemoteStorage(coreconfig.Datadog.GetBool("runtime_security_config.sbom.remote_storage.compression"))
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate SBOM remote storage: %w", err)
	}

	return &RuntimeSecurityAgent{
		client:               client,
		hostname:             hostname,
		telemetry:            telemetry,
		storage:              storage,
		running:              atomic.NewBool(false),
		connected:            atomic.NewBool(false),
		eventReceived:        atomic.NewUint64(0),
		activityDumpReceived: atomic.NewUint64(0),
		sbomReceived:         atomic.NewUint64(0),
		sbomRemoteStorage:    sbomRemoteStorage,
	}, nil
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start(reporter event.Reporter, endpoints *config.Endpoints) {
	rsa.reporter = reporter
	rsa.endpoints = endpoints

	ctx, cancel := context.WithCancel(context.Background())
	rsa.cancel = cancel

	rsa.running.Store(true)
	// Start the system-probe events listener
	go rsa.StartEventListener()
	// Start activity dumps listener
	go rsa.StartActivityDumpListener()
	// Start sbom listener
	go rsa.StartSBOMListener()
	// Send Runtime Security Agent telemetry
	go rsa.telemetry.run(ctx, rsa)
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

// StartSBOMListener starts listening for new SBOM from system-probe
func (rsa *RuntimeSecurityAgent) StartSBOMListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	for rsa.running.Load() {
		stream, err := rsa.client.GetSBOMStream()
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
			log.Infof("Got SBOM for '%s'", msg.GetContainerID())

			rsa.sbomReceived.Inc()

			// Dispatch SBOM
			rsa.DispatchSBOM(msg)
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
	dump, err := probe.NewActivityDumpFromMessage(msg.GetDump())
	if err != nil {
		log.Errorf("%v", err)
		return
	}

	raw := bytes.NewBuffer(msg.GetData())

	for _, requests := range dump.StorageRequests {
		if err := rsa.storage.PersistRaw(requests, dump, raw); err != nil {
			log.Errorf("%v", err)
		}
	}
}

// DispatchSBOM forwards an SBOM message to the backend
func (rsa *RuntimeSecurityAgent) DispatchSBOM(msg *api.SBOMMessage) {
	if err := rsa.sbomRemoteStorage.SendSBOM(msg); err != nil {
		log.Errorf("couldn't sent SBOM: %v", err)
	}
}

// GetStatus returns the current status on the agent
func (rsa *RuntimeSecurityAgent) GetStatus() map[string]interface{} {
	base := map[string]interface{}{
		"connected":            rsa.connected.Load(),
		"eventReceived":        rsa.eventReceived.Load(),
		"activityDumpReceived": rsa.activityDumpReceived.Load(),
		"sbomReceived":         rsa.sbomReceived.Load(),
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
