// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package utils

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

// UseEndpointSlices returns true if the Agent config has enabled endpoint slices.
// A fallback mechanism isn't possible in this case because the kube api server isn't
// available to this agent.
func UseEndpointSlices() bool {
	return pkgconfigsetup.Datadog().GetBool("kubernetes_use_endpoint_slices")
}
