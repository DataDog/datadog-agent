// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	payload "github.com/DataDog/agent-payload/v5/process"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewProcessEventsCheck returns an instance of the ProcessEventsCheck.
func NewProcessEventsCheck(config ddconfig.ConfigReader) *ProcessEventsCheck {
	return &ProcessEventsCheck{
		config: config,
	}
}

// ProcessEventsCheck collects process lifecycle events such as exec and exit signals
type ProcessEventsCheck struct {
	initMutex sync.Mutex

	config ddconfig.ConfigReader

	store    events.Store
	listener *events.SysProbeListener
	hostInfo *HostInfo

	maxBatchSize int
}

// Init initializes the ProcessEventsCheck.
func (e *ProcessEventsCheck) Init(_ *SysProbeConfig, info *HostInfo) error {
	e.initMutex.Lock()
	defer e.initMutex.Unlock()

	if e.store != nil || e.listener != nil {
		return log.Error("process_events check has already been initialized")
	}

	log.Info("Initializing process_events check")
	e.hostInfo = info
	e.maxBatchSize = getMaxBatchSize(e.config)

	store, err := events.NewRingStore(e.config, statsd.Client)
	if err != nil {
		log.Errorf("RingStore can't be created: %v", err)
		return err
	}
	e.store = store

	listener, err := events.NewListener(func(e *model.ProcessEvent) {
		// push events to the store asynchronously without checking for errors
		_ = store.Push(e, nil)
	})
	if err != nil {
		log.Errorf("Event Listener can't be created: %v", err)
		return err
	}
	e.listener = listener

	e.start()
	log.Info("process_events check correctly set up")
	return nil
}

// start kicks off process lifecycle events collection and keep them in memory until they're fetched in the next check run
func (e *ProcessEventsCheck) start() {
	e.store.Run()
	e.listener.Run()
}

// IsEnabled returns true if the check is enabled by configuration
func (e *ProcessEventsCheck) IsEnabled() bool {
	return e.config.GetBool("process_config.event_collection.enabled")
}

// SupportsRunOptions returns true if the check supports RunOptions
func (e *ProcessEventsCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ProcessEventsCheck.
func (e *ProcessEventsCheck) Name() string { return ProcessEventsCheckName }

// Realtime returns a value that says whether this check should be run in real time.
func (e *ProcessEventsCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (e *ProcessEventsCheck) ShouldSaveLastRun() bool { return true }

// Run fetches process lifecycle events that have been stored in-memory since the last check run
func (e *ProcessEventsCheck) Run(nextGroupID func() int32, _ *RunOptions) (RunResult, error) {
	if !e.isCheckCorrectlySetup() {
		return nil, errors.New("the process_events check hasn't been correctly initialized")
	}

	ctx := context.Background()
	events, err := e.store.Pull(ctx, time.Second)
	if err != nil {
		return nil, fmt.Errorf("can't pull events from the Event Store: %v", err)
	}

	payloadEvents := FmtProcessEvents(events)
	chunks := chunkProcessEvents(payloadEvents, e.maxBatchSize)

	messages := make([]payload.MessageBody, len(chunks))
	groupID := nextGroupID()
	for c, chunk := range chunks {
		messages[c] = &payload.CollectorProcEvent{
			Hostname:  e.hostInfo.HostName,
			Info:      e.hostInfo.SystemInfo,
			Events:    chunk,
			GroupId:   groupID,
			GroupSize: int32(len(chunks)),
		}
	}

	return StandardRunResult(messages), nil
}

// Cleanup frees any resource held by the ProcessEventsCheck before the agent exits
func (e *ProcessEventsCheck) Cleanup() {
	log.Info("Cleaning up process_events check")
	if e.listener != nil {
		e.listener.Stop()
	}

	if e.store != nil {
		e.store.Stop()
	}
	log.Info("process_events check cleaned up")
}

func (e *ProcessEventsCheck) isCheckCorrectlySetup() bool {
	return e.store != nil && e.listener != nil
}

// chunkProcessEvents splits a list of ProcessEvents into chunks according to the given chunk size
// TODO: Move it to chunker
func chunkProcessEvents(events []*payload.ProcessEvent, size int) [][]*payload.ProcessEvent {
	chunkCount := len(events) / size
	if chunkCount*size < len(events) {
		chunkCount++
	}
	chunks := make([][]*payload.ProcessEvent, 0, chunkCount)

	for i := 0; i < len(events); i += size {
		end := i + size
		if end > len(events) {
			end = len(events)
		}
		chunks = append(chunks, events[i:end])
	}

	return chunks
}
