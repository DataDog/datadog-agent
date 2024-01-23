// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver || kubelet

package kubernetes

import (
	"context"
)

// GetKubeAPIServerHostname returns the hostname from kubeapiserver
func GetKubeAPIServerHostname(context.Context) (string, error) {
	panic("not called")
}
