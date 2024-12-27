// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
	"time"
)

func TestSyncThrottler(_ *testing.T) {
	throtler := NewSyncThrottler(3)

	t1 := throtler.RequestToken()
	t2 := throtler.RequestToken()
	t3 := throtler.RequestToken()

	go func() {
		time.Sleep(1 * time.Second)
		throtler.Release(t3)
	}()

	t4 := throtler.RequestToken() // this should block until token t3 is released
	throtler.Release(t4)

	throtler.Release(t4) // releasing a token that was already released should be ok (idempotent)

	throtler.Release(t1)
	throtler.Release(t2)
}
