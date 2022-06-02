// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package checks

import (
	"context"
	"errors"
	"fmt"
	"time"

	payload "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/events"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessEvents is a ProcessEventsCheck singleton
var ProcessEvents = &ProcessEventsCheck{}

// ProcessEventsCheck collects process lifecycle events such as exec and exit signals
type ProcessEventsCheck struct {
	store    events.Store
	listener *events.SysProbeListener
	sysInfo  *payload.SystemInfo

	maxBatchSize int
}

// Init initializes the ProcessEventsCheck.
func (e *ProcessEventsCheck) Init(_ *config.AgentConfig, info *payload.SystemInfo) {
	log.Info("Initializing process_events check")
	e.sysInfo = info
	e.maxBatchSize = getMaxBatchSize()

	store, err := events.NewRingStore(statsd.Client)
	if err != nil {
		log.Errorf("RingStore can't be created: %v", err)
		return
	}
	e.store = store

	listener, err := events.NewListener(func(e *model.ProcessEvent) {
		// push events to the store asynchronously without checking for errors
		_ = store.Push(e, nil)
	})
	if err != nil {
		log.Errorf("Event Listener can't be created: %v", err)
		return
	}
	e.listener = listener

	e.start()
	log.Info("process_events check correctly set up")
}

// start kicks off process lifecycle events collection and keep them in memory until they're fetched in the next check run
func (e *ProcessEventsCheck) start() {
	e.store.Run()
	e.listener.Run()
}

// Name returns the name of the ProcessEventsCheck.
func (e *ProcessEventsCheck) Name() string { return config.ProcessEventsCheckName }

// RealTime returns a value that says whether this check should be run in real time.
func (e *ProcessEventsCheck) RealTime() bool { return false }

// Run fetches process lifecycle events that have been stored in-memory since the last check run
func (e *ProcessEventsCheck) Run(cfg *config.AgentConfig, groupID int32) ([]payload.MessageBody, error) {
	if !e.IsCheckCorrectlySetup() {
		return nil, errors.New("the process_events check hasn't been correctly initialized")
	}

	ctx := context.Background()
	events, err := e.store.Pull(ctx, time.Second)
	if err != nil {
		return nil, fmt.Errorf("can't pull events from the Event Store: %v", err)
	}

	payloadEvents := fmtProcessEvents(events)
	chunks := chunkProcessEvents(payloadEvents, e.maxBatchSize)

	messages := make([]payload.MessageBody, len(chunks))
	for c, chunk := range chunks {
		messages[c] = &payload.CollectorProcEvent{
			Hostname:  cfg.HostName,
			Info:      e.sysInfo,
			Events:    chunk,
			GroupId:   groupID,
			GroupSize: int32(len(chunks)),
		}
	}

	return messages, nil
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

func (e *ProcessEventsCheck) IsCheckCorrectlySetup() bool {
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

func fmtProcessEvents(events []*model.ProcessEvent) []*payload.ProcessEvent {
	payloadEvents := make([]*payload.ProcessEvent, 0, len(events))

	for _, e := range events {
		pE := &payload.ProcessEvent{
			CollectionTime: e.CollectionTime.UnixNano(),
			Pid:            e.Pid,
			Command: &payload.Command{
				Exe:  e.Exe,
				Args: e.Cmdline,
				Ppid: int32(e.Ppid),
			},
			User: &payload.ProcessUser{
				Name: e.Username,
				Uid:  int32(e.UID),
				Gid:  int32(e.GID),
			},
		}

		switch e.EventType {
		case model.Exec:
			pE.Type = payload.ProcEventType_exec
			exec := &payload.ProcessExec{
				ForkTime: e.ForkTime.UnixNano(),
				ExecTime: e.ExecTime.UnixNano(),
			}
			pE.TypedEvent = &payload.ProcessEvent_Exec{Exec: exec}
		case model.Exit:
			pE.Type = payload.ProcEventType_exit
			exit := &payload.ProcessExit{
				ExecTime: e.ExecTime.UnixNano(),
				ExitTime: e.ExitTime.UnixNano(),
				ExitCode: 0,
			}
			pE.TypedEvent = &payload.ProcessEvent_Exit{Exit: exit}
		default:
			log.Error("Unexpected event type, dropping it")
			continue
		}

		payloadEvents = append(payloadEvents, pE)
	}

	return payloadEvents
}
