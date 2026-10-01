// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

// newMethodGate returns the set of task methods the macOS daemon executes.
//
// Configuration management is implemented, so the three *_config methods are executed. The three
// version methods are still declined: nothing on macOS can download and lay down a package
// version yet, and a stub that returned nil would be reported to the backend as done.
func newMethodGate() methodGate {
	return supportedMethods{
		methodStartConfigExperiment:   true,
		methodStopConfigExperiment:    true,
		methodPromoteConfigExperiment: true,
	}
}
