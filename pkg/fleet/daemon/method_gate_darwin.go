// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

// newMethodGate returns the set of task methods the macOS daemon executes.
//
// Empty: the daemon reports host state and receives task sets, but it executes nothing yet, so
// every method is declined. The three *_config methods are turned on with configuration
// management; the three version methods with update management.
func newMethodGate() methodGate {
	return supportedMethods{}
}
