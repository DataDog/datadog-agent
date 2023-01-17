// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

// starter starts a set of components in series.
type starter struct {
	components []Startable
}

var _ Starter = &starter{}
var _ Startable = &starter{}

// NewStarter returns a new serial starter.
//
// The Start() method of this object will start all components, one
// by one, in the order they were added.
//
// Any components included in the arguments will be included in the
// set of components, as if starter.Add(..) had been called for each.
func NewStarter(components ...Startable) Starter {
	return &starter{
		components: components,
	}
}

// Add implements Starter#Add.
func (s *starter) Add(components ...Startable) {
	s.components = append(s.components, components...)
}

// Start implements Startable#Start.
func (s *starter) Start() {
	for _, c := range s.components {
		c.Start()
	}
}
