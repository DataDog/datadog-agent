// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// applyRoutingHeaders sets x-dd-logs-routing on each endpoint based on its own
// transport: "grpc" for gRPC endpoints, "http" for HTTP endpoints.
// Nodeless endpoints get no routing header so their traffic goes to "elsewhere".
func applyRoutingHeaders(endpoints *config.Endpoints) {
	if endpoints.Nodeless {
		delete(endpoints.Main.ExtraHTTPHeaders, "x-dd-logs-routing")
		for i := range endpoints.Endpoints {
			delete(endpoints.Endpoints[i].ExtraHTTPHeaders, "x-dd-logs-routing")
		}
		return
	}

	var mainRoutingValue string
	if endpoints.UseGRPC {
		mainRoutingValue = "grpc"
	} else if endpoints.UseHTTP {
		mainRoutingValue = "http"
	} else {
		return
	}

	if endpoints.Main.ExtraHTTPHeaders == nil {
		endpoints.Main.ExtraHTTPHeaders = map[string]string{}
	}
	endpoints.Main.ExtraHTTPHeaders["x-dd-logs-routing"] = mainRoutingValue

	for i := range endpoints.Endpoints {
		ep := &endpoints.Endpoints[i]
		if ep.ExtraHTTPHeaders == nil {
			ep.ExtraHTTPHeaders = map[string]string{}
		}
		if ep.UseGRPC {
			ep.ExtraHTTPHeaders["x-dd-logs-routing"] = "grpc"
		} else {
			ep.ExtraHTTPHeaders["x-dd-logs-routing"] = mainRoutingValue
		}
	}
}
