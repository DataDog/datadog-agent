// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package api

import (
	"context"
	"sync/atomic"

	v1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
)

// clusterChecks is set once the cluster check handler starts, after the API
// server.
var clusterChecks atomic.Pointer[ClusterCheckNodes]

func startKubeMetadataStreamer(ctx context.Context, wmeta workloadmeta.Component) kubeMetadataStreamer {
	srv := v1.NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), wmeta)
	srv.SetKueueCheckNodesProvider(func() (map[string]struct{}, bool) {
		handler := clusterChecks.Load()
		if handler == nil {
			return nil, false
		}
		return (*handler).NodesRunningCheck(v1.KueueCheckName)
	})
	srv.Start(ctx)
	return srv
}

// SetKubeMetadataClusterChecks lets the kube metadata stream know which nodes
// run a Kueue check, from the cluster check dispatcher.
func SetKubeMetadataClusterChecks(handler ClusterCheckNodes) {
	clusterChecks.Store(&handler)
}
