// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import (
	"sync"
)

// parallelStopper implements the logic to stop different components from a data pipeline in parallel
type parallelStopper struct {
	components []Stoppable
}

// NewParallelStopper returns a new parallelStopper
func NewParallelStopper(components ...Stoppable) Stopper {
	return &parallelStopper{
		components: components,
	}
}

// Add appends new elements to the array of components to stop
func (g *parallelStopper) Add(components ...Stoppable) {
	g.components = append(g.components, components...)
}

// Stop stops all components in parallel and returns when they are all stopped
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
