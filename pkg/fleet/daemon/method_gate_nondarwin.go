// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package daemon

// newMethodGate returns the set of task methods the daemon executes.
//
// Linux and Windows implement all of them, so the gate never declines and behaviour is unchanged.
func newMethodGate() methodGate {
	gate := make(supportedMethods, len(allMethods))
	for _, method := range allMethods {
		gate[method] = true
	}
	return gate
}
