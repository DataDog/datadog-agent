// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package signals

import "github.com/DataDog/datadog-agent/comp/core/stopper"

// This is a new package in order to avoid cyclical imports

// TODO: once everything is using comp/core/stopper, remove this entire
// package.

var (
	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// ErrorStopper is the channel used by other packages to ask for stopping the agent because of an error
	ErrorStopper = make(chan bool)
)

// Stop stops the running agent, optionally with an error.  For an expected
// stop, use Stop(nil).
//
// This function returns immediately, and shutdown begins in other goroutines.
func Stop(err error) {
	// if there's a stopper component active, then use that
	if stopper.RunningInstance != nil {
		stopper.RunningInstance.Stop(err)
		return
	}

	if err != nil {
		ErrorStopper <- true
	} else {
		Stopper <- true
	}
}
