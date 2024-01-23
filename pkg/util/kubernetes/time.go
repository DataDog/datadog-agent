// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"time"
)

// TimeWithoutWall fixes the `wall` issue in unit tests.
// THIS FUNCTION SHOULD NOT BE USED OUTSIDE OF TESTS.
// Unstructured serializes time to string in RFC3339 without Nano seconds.
// when it's parsed back, the Go time.Time does not have the `wall` field as it's used for nanosecs.
func TimeWithoutWall(t time.Time) time.Time {
	panic("not called")
}
