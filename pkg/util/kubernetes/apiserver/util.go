// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/tools/cache"
)

// syncTimeout can be used to wait for the kubernetes client-go cache to sync.
var syncTimeout = config.Datadog.GetDuration("cache_sync_timeout") * time.Second

// SyncInformers should be called after the instanciation of new informers.
// It's blocking until the informers are synced or the timeout exceeded.
func SyncInformers(informers []cache.SharedInformer) error {
	var g errgroup.Group
	for _, inf := range informers {
		g.Go(func() error {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(syncTimeout))
			defer cancel()
			if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
				return log.Error("cache sync timed out")
			}
			return nil
		})
	}
	return g.Wait()
}
