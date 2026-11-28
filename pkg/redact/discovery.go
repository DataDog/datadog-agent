// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	discoveryv1 "k8s.io/api/discovery/v1"
)

// ScrubEndpointSlice scrubs the metadata of a EndpointSlice.
func ScrubEndpointSlice(r *discoveryv1.EndpointSlice, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}
