// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import (
	"sync"
)

// parallelStopper stops a set of components in parallel.
type parallelStopper struct {
	components []Stoppable
}

var _ Stopper = &parallelStopper{}
var _ Stoppable = &parallelStopper{}

// NewParallelStopper returns a new parallel stopper.
//
// The Stop() method of this object will stop all components concurrently,
// calling each component's Stop method in a dedicated goroutine.  It will
// return only when all Stop calls have completed.
//
// Any components included in the arguments will be included in the
// set of components, as if stopper.Add(..) had been called for each.
func NewParallelStopper(components ...Stoppable) Stopper {
	return &parallelStopper{
		components: components,
	}
}

// Add implements Stopper#Add.
func (g *parallelStopper) Add(components ...Stoppable) {
	g.components = append(g.components, components...)
}

// Stop implements Stoppable#Stop.
func (g *parallelStopper) Stop() {
	wg := &sync.WaitGroup{}
	for _, component := range g.components {
		wg.Add(1)
		go func(s Stoppable) {
			s.Stop()
			wg.Done()
		}(component)
	}
	wg.Wait()
}
