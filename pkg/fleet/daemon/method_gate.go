// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"errors"
	"fmt"
)

// errMethodNotSupported is returned for a task method this platform declines to execute.
var errMethodNotSupported = errors.New("method not supported on this platform")

// methodGate decides which remote task methods the daemon will execute.
//
// The task protocol has no way to report a method as merely unimplemented. handleUpdaterTaskUpdate
// acknowledges every request it dispatches, so an unimplemented method reaching a stub that
// returns nil would be reported as done -- a false statement the backend would then act on by
// advancing the deployment. The gate sits ahead of dispatch and declines instead: the request gets
// an error status and the rest of the task set is unaffected.
//
// The gate is data rather than compile-time branching, so turning a method on is one line and the
// set is directly assertable in a test.
type methodGate interface {
	// Supported reports whether the daemon will execute the method.
	Supported(method string) bool
	// Decline returns the error to report for a method the daemon will not execute.
	Decline(method string) error
}

// supportedMethods is a methodGate over the set of method names the daemon executes.
type supportedMethods map[string]bool

// Supported reports whether the daemon will execute the method.
func (s supportedMethods) Supported(method string) bool {
	return s[method]
}

// Decline returns the error to report for a method the daemon will not execute.
func (s supportedMethods) Decline(method string) error {
	return fmt.Errorf("%w: %s", errMethodNotSupported, method)
}

// allMethods is every task method the backend can send.
var allMethods = []string{
	methodInstallPackage,
	methodUninstallPackage,
	methodStartExperiment,
	methodStopExperiment,
	methodPromoteExperiment,
	methodStartConfigExperiment,
	methodStopConfigExperiment,
	methodPromoteConfigExperiment,
}

// newMethodGate returns the set of task methods the daemon executes.
//
// Every platform now implements all eight, so the gate never declines. It is kept rather than
// deleted because it is the mechanism, not the policy: it is what stands between an unimplemented
// method and an acknowledgement, and the next platform or method to arrive incomplete needs it to
// already be here. Until Part 3 macOS declined the three version methods from a
// method_gate_darwin.go; that file is gone because macOS carries them out now.
func newMethodGate() methodGate {
	gate := make(supportedMethods, len(allMethods))
	for _, method := range allMethods {
		gate[method] = true
	}
	return gate
}
