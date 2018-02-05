// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package restart

// serialGroup implements the logic to stop different components from a data pipeline in series
type serialGroup struct {
	stoppers []Stopper
}

// NewSerialGroup returns a new serialGroup
func NewSerialGroup(stoppers ...Stopper) Group {
	return &serialGroup{
		stoppers: stoppers,
	}
}

// Add appends new elements to the array of components to stop
func (g *serialGroup) Add(stoppers ...Stopper) {
	g.stoppers = append(g.stoppers, stoppers...)
}

// Stop stops all components one after another
func (g *serialGroup) Stop() {
	for _, stopper := range g.stoppers {
		stopper.Stop()
	}
}
