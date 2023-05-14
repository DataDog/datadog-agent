// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startstop provides useful functionality for starting and stopping agent
// components.
//
// The Startable and Stoppable interfaces define components that can be started and
// stopped, respectively.  The package then provides utility functionality to start
// and stop components either concurrently or in series.
package startstop

// StartStoppable represents a startable and stopable object
type StartStoppable interface {
	Startable
	Stoppable
}
