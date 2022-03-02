// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

// Starter implements the logic to start different components from a data pipeline in series
type starter struct {
	components []Startable
}

// NewStarter returns a new starter
func NewStarter(components ...Startable) Starter {
	return &starter{
		components: components,
	}
}

// Add appends new elements to the array of components to start
func (s *starter) Add(components ...Startable) {
	s.components = append(s.components, components...)
}

// Start starts all components one after another
func (s *starter) Start() {
	for _, c := range s.components {
		c.Start()
	}
}
