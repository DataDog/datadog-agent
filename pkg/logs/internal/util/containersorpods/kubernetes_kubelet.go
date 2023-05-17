// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package containersorpods

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// kubernetesReady checks if the kubelet service is ready, returning a duration
// after which it should be re-checked if not.
func kubernetesReady() (bool, time.Duration) {
	util, retrier := kubelet.GetKubeUtilWithRetrier()
	if util != nil {
		return true, 0
	}
	return false, time.Until(retrier.NextRetry())
}
