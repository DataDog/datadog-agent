// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package restart

// Stopper represents a stoppable object
type Stopper interface {
	Stop()
}

// Group represents a group of stoppable objects from a data pipeline
type Group interface {
	Stopper
	Add(stoppers ...Stopper)
}
