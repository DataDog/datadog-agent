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

// eventWithData is an event record with rendered system values
type eventWithData struct {
	winevent   *evtapi.EventRecord
	systemVals evtapi.EvtVariantValues
	evtapi     evtapi.API
}

// Close frees resources associated with the event
func (e *eventWithData) Close() {
	if e.systemVals != nil {
		e.systemVals.Close()
	}
	if e.winevent != nil {
		evtapi.EvtCloseRecord(e.evtapi, e.winevent.EventRecordHandle)
	}
}

type eventDataGetter struct {
	doneCh <-chan struct{}
	inCh   <-chan *evtapi.EventRecord
	outCh  chan<- *eventWithData

	// contexts
	evtapi              evtapi.API
	systemRenderContext evtapi.EventRenderContextHandle
}

func (f *eventDataGetter) run(w *sync.WaitGroup) {
	defer w.Done()
	defer close(f.outCh)
	for winevent := range f.inCh {
		e, err := f.getEventData(winevent)
		if err != nil {
			log.Errorf("%v", err)
			continue
		}

		select {
		case f.outCh <- e:
		case <-f.doneCh:
			return
		}
	}
}

func (f *eventDataGetter) getEventData(winevent *evtapi.EventRecord) (*eventWithData, error) {
	e := &eventWithData{
		winevent: winevent,
		evtapi:   f.evtapi,
	}

	// Render the values
	vals, err := f.evtapi.EvtRenderEventValues(f.systemRenderContext, winevent.EventRecordHandle)
	if err != nil {
		return nil, fmt.Errorf("failed to render values: %w", err)
	}

	// transfer ownership of vals to the event
	e.systemVals = vals

	return e, nil
}
