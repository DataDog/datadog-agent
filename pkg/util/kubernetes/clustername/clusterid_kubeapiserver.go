// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package clustername

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// Overridable in tests to exercise flavor routing without initializing the
// process-wide Kubernetes client.
var getClusterAgentClusterID = func() (string, error) {
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return "", err
	}
	return common.ResolveClusterID(context.TODO(), client.Cl.CoreV1())
}
