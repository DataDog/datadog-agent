// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

type eventDataFilter struct {
	doneCh <-chan struct{}
	inCh   <-chan *eventWithData
	outCh  chan<- *eventWithData

	// config
	eventIDs []uint16
}

func (f *eventDataFilter) run(w *sync.WaitGroup) {
	defer w.Done()
	defer close(f.outCh)
	for e := range f.inCh {
		exclude, err := f.filterEvent(e)
		if err != nil {
			log.Errorf("error filtering event: %v", err)
			e.Close()
			continue
		}
		if exclude {
			e.Close()
			continue
		}

		select {
		case f.outCh <- e:
		case <-f.doneCh:
			return
		}
	}
}

func (f *eventDataFilter) filterEvent(e *eventWithData) (bool, error) {
	// Get the event ID
	eventID, err := e.systemVals.UInt(evtapi.EvtSystemEventID)
	if err != nil {
		return false, fmt.Errorf("error getting event ID: %v", err)
	}

	// Check if the event ID is in the list of allowed event IDs
	if f.isAllowedEventID(uint16(eventID)) {
		return false, nil
	}

	// event will be excluded
	return true, nil
}

func (f *eventDataFilter) isAllowedEventID(eventID uint16) bool {
	for _, id := range f.eventIDs {
		if eventID == id {
			return true
		}
	}
	return false
}
