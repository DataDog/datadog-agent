// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package restart

// Starter represents a startable object
type Starter interface {
	Start()
}

// Start starts all starters in series
func Start(starters ...Starter) {
	for _, starter := range starters {
		starter.Start()
	}
}
