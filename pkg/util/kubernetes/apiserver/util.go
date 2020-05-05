// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"context"
	"fmt"
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
func SyncInformers(informers map[InformerName]cache.SharedInformer) error {
	var g errgroup.Group
	for name := range informers {
		name := name // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			defer cancel()
			start := time.Now()
			if !cache.WaitForCacheSync(ctx.Done(), informers[name].HasSynced) {
				return fmt.Errorf("couldn't sync informer %s in %v", name, time.Now().Sub(start))
			}
			log.Debugf("Sync done for informer %s in %v", name, time.Now().Sub(start))
			return nil
		})
	}
	return g.Wait()
}
