// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type TestJobCheck struct {
	TestCheck
	id string
}

func (c *TestJobCheck) ID() checkid.ID { return checkid.ID(c.id) }

func TestBucket_RemoveJob(t *testing.T) {
	bucket := &jobBucket{}

	// add 2 dummy checks
	bucket.addJob(&TestJobCheck{id: "1"})
	bucket.addJob(&TestJobCheck{id: "2"})
	require.Equal(t, 2, bucket.size())

	// Add a check with a finalizer to the bucket, then remove it
	finalized := make(chan struct{}, 1)
	checkWithFinalizer := &TestJobCheck{id: "withFinalizer"}
	runtime.SetFinalizer(checkWithFinalizer, func(c *TestJobCheck) {
		finalized <- struct{}{}
	})
	bucket.addJob(checkWithFinalizer)
	require.Equal(t, 3, bucket.size())
	bucket.removeJob(checkWithFinalizer.ID())
	checkWithFinalizer = nil // make sure we don't keep any reference to the check

	require.Equal(t, 2, bucket.size())

	// Trigger a full GC run, which should GC the check. Then leave 10 seconds for
	// the runtime to run the finalizer.
	// If the finalizer hasn't run in this timeframe, it probably means that the bucket
	// still keeps a reference to the check in its internal data structures.
	runtime.GC()
	testutil.AssertTrueBeforeTimeout(
		t,
		100*time.Millisecond,
		10*time.Second,
		func() bool {
			select {
			case <-finalized:
				return true
			default:
				return false
			}
		},
	)

	// use the bucket, just to keep it alive during the earlier GC run
	bucket.addJob(&TestJobCheck{id: "here so the GC doesn't GC the entire bucket"})
}
