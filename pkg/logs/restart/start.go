// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package restart

// Startable represents a startable object
type Startable interface {
	Start()
}

// Start starts all components in series
func Start(components ...Startable) {
	for _, component := range components {
		component.Start()
	}
}
