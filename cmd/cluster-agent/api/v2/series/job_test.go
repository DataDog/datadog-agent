// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package series

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/DataDog/agent-payload/v5/gogen"
	"k8s.io/client-go/util/workqueue"
)

func TestIdleJobQueueStopsOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[*gogen.MetricPayload]())
		jq := &jobQueue{taskQueue: queue}
		done := make(chan struct{})
		go func() { jq.start(ctx); close(done) }()
		synctest.Wait()
		cancel()
		<-done
		jq.start(ctx)
		jq.m.Lock()
		jq.m.Unlock()
	})
}
