// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package clustername

import "errors"

var getClusterAgentClusterID = func() (string, error) {
	return "", errors.New("Cluster Agent cluster ID resolution requires kubeapiserver support")
}
