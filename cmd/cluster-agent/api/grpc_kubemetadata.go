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
	instrumentationtargets "github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation/targets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func startKubeMetadataStreamer(ctx context.Context, wmeta workloadmeta.Component) kubeMetadataStreamer {
	var resolver *instrumentationtargets.Resolver
	if pkgconfigsetup.Datadog().GetBool("instrumentation_crd_controller.enabled") {
		registry, err := instrumentationtargets.NewRegistry(pkgconfigsetup.Datadog())
		if err != nil {
			log.Warnf("Invalid custom DatadogInstrumentation workload target configuration: %v", err)
		}
		if registry.HasCustomTargets() {
			apiClient, clientErr := apiserver.GetAPIClient()
			if clientErr != nil {
				log.Warnf("DatadogInstrumentation workload target resolution is unavailable: %v", clientErr)
			} else {
				resolver = instrumentationtargets.NewResolver(registry, apiClient.DynamicCl)
			}
		}
	}

	srv := v1.NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), wmeta, resolver)
	srv.Start(ctx)
	return srv
}
