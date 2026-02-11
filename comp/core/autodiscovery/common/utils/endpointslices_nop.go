// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package utils

// UseEndpointSlices returns false because the EndpointSlices API is not available
// without access to the API server.
func UseEndpointSlices() bool {
	return false
}
