// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package api

import (
	"context"

	v1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
)

func startKubeMetadataStreamer(ctx context.Context, wmeta workloadmeta.Component, waitPodCollectionSynced func(context.Context) bool) kubeMetadataStreamer {
	var opts []v1.KubeMetadataStreamServerOption
	if waitPodCollectionSynced != nil {
		opts = append(opts, v1.WithPerNodeWorkloadFiltering(waitPodCollectionSynced))
	}
	srv := v1.NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), wmeta, opts...)
	srv.Start(ctx)
	return srv
}
