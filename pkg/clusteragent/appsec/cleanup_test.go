// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestCleanupLeadershipWaitCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		changes := make(chan struct{})
		done := make(chan bool, 1)
		go func() { done <- waitForCleanupLeadership(ctx, changes, func() bool { return false }) }()
		synctest.Wait()
		cancel()
		if <-done {
			t.Fatal("canceled cleanup acquired leadership")
		}
	})
}
