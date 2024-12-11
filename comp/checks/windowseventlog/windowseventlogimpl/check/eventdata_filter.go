// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog/windowseventlogimpl/check/eventdatafilter"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type eventDataFilter struct {
	doneCh <-chan struct{}
	inCh   <-chan *eventWithData
	outCh  chan<- *eventWithData

	filter eventdatafilter.Filter
}

func (f *eventDataFilter) run(w *sync.WaitGroup) {
	defer w.Done()
	defer close(f.outCh)
	for e := range f.inCh {
		include, err := f.includeEvent(e)
		if err != nil {
			log.Errorf("error filtering event: %v", err)
			e.Close()
			continue
		}
		if !include {
			e.Close()
			continue
		}

		select {
		case f.outCh <- e:
		case <-f.doneCh:
			e.Close()
			return
		}
	}
}

func (f *eventDataFilter) includeEvent(e *eventWithData) (bool, error) {
	return f.filter.Match(e)
}
