// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package restart

import (
	"sync"
)

// parallelGroup implements the logic to stop different components from a data pipeline in parallel
type parallelGroup struct {
	stoppers []Stopper
}

// NewParallelGroup returns a new parallelGroup
func NewParallelGroup(stoppers ...Stopper) Group {
	return &parallelGroup{
		stoppers: stoppers,
	}
}

// Add appends new elements to the array of components to stop
func (g *parallelGroup) Add(stoppers ...Stopper) {
	g.stoppers = append(g.stoppers, stoppers...)
}

// Stop stops all components in parallel and returns when they are all stopped
func (g *parallelGroup) Stop() {
	wg := &sync.WaitGroup{}
	for _, stopper := range g.stoppers {
		wg.Add(1)
		go func(s Stopper) {
			s.Stop()
			wg.Done()
		}(stopper)
	}
	wg.Wait()
}
