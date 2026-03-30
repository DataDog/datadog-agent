// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// UseEndpointSlices checks if the Agent should use the EndpointSlices API.
// It checks that both the config enables EndpointSlices AND the Kubernetes cluster version
// supports them (v1.21+), falling back to Endpoints if the cluster is too old.
func UseEndpointSlices() bool {
	return apiserver.UseEndpointSlices()
}
