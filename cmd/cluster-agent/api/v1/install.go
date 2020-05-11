// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package v1

import (
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

var (
	apiRequests = telemetry.NewCounterWithOpts("", "api_requests",
		[]string{"handler", "status"}, "Counter of requests made to the cluster agent API.",
		telemetry.Options{NoDoubleUnderscoreSep: true})
)

func incrementRequestMetric(handler string, status int) {
	apiRequests.Inc(handler, strconv.Itoa(status))
}

// Install registers v1 API endpoints
func Install(r *mux.Router, sc clusteragent.ServerContext) {
	if config.Datadog.GetBool("cloud_foundry") {
		installCloudFoundryMetadataEndpoints(r)
	} else {
		installKubernetesMetadataEndpoints(r)
	}
	installClusterCheckEndpoints(r, sc)
	installEndpointsCheckEndpoints(r, sc)
}
