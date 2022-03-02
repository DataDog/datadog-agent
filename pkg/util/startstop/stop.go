// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

// Stoppable represents a stoppable object
type Stoppable interface {
	Stop()
}

// Stopper stops a group of stoppable objects from a data pipeline
type Stopper interface {
	Stoppable
	Add(components ...Stoppable)
}
