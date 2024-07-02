// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"testing"
	"time"
)

func TestTimer(t *testing.T) {
	var myTimer timer = realTime{}
	val := myTimer.Now()
	compare := time.Now()
	// should be basically the same time, within a millisecond
	if compare.Sub(val).Truncate(time.Millisecond) != 0 {
		t.Errorf("expected within a millisecond: %v, %v", compare, val)
	}
}
