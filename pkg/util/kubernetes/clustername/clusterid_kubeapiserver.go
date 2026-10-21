// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package clustername

import "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

// Tests can replace this function. Then they do not need the process-wide Kubernetes client.
var getClusterAgentClusterID = apiserver.GetClusterID
