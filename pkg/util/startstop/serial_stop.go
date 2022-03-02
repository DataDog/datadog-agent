// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

// serialStopper implements the logic to stop different components from a data pipeline in series
type serialStopper struct {
	components []Stoppable
}

// NewSerialStopper returns a new serialGroup
func NewSerialStopper(components ...Stoppable) Stopper {
	return &serialStopper{
		components: components,
	}
}

// Add appends new elements to the array of components to stop
func (g *serialStopper) Add(components ...Stoppable) {
	g.components = append(g.components, components...)
}

// Stop stops all components one after another
func (g *serialStopper) Stop() {
	for _, stopper := range g.components {
		stopper.Stop()
	}
}
