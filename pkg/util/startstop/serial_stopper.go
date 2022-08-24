// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

// serialStopper stops a set of components in series.
type serialStopper struct {
	components []Stoppable
}

var _ Stopper = &serialStopper{}
var _ Stoppable = &serialStopper{}

// NewSerialStopper returns a new serial stopper.
//
// The Stop() method of this object will stop all components, one
// by one, in the order they were added.
//
// Any components included in the arguments will be included in the
// set of components, as if stopper.Add(..) had been called for each.
func NewSerialStopper(components ...Stoppable) Stopper {
	return &serialStopper{
		components: components,
	}
}

// Add implements Stopper#Add.
func (g *serialStopper) Add(components ...Stoppable) {
	g.components = append(g.components, components...)
}

// Add implements Stoppable#Stop.
func (g *serialStopper) Stop() {
	for _, stopper := range g.components {
		stopper.Stop()
	}
}
