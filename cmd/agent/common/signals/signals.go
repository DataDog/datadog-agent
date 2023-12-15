// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package signals defines global agent signal channels
package signals

var (
	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// ErrorStopper is the channel used by other packages to ask for stopping the agent because of an error
	ErrorStopper = make(chan bool)
)
