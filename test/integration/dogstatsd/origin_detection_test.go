// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package dogstatsd

import "testing"

// FIXME: move as a system test once the runner is able to run them

// TestUDSOriginDetection ensures UDS origin detection works, by submitting
// a metric from a `socat` container. As we need the origin PID to stay running,
// we can't just `netcat` to the socket, that's why we keep socat running as
// UDP->UDS proxy and submit the metric through it.
//
// FIXME: this test should be ported to the go docker client
func TestUDSOriginDetection(t *testing.T) {
	testUDSOriginDetection(t)
}
