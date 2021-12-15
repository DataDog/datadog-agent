// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet
// +build kubelet

package util

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func isAgentKubeHostNetwork() (bool, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return true, err
	}

	return ku.IsAgentHostNetwork(context.TODO())
}
