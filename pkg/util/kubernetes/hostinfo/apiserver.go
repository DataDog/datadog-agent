// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package hostinfo

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func apiserverNodeLabels(nodeName string) (map[string]string, error) {
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, err
	}
	return client.NodeLabels(nodeName)
}
