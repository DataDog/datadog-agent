// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func isAgentKubeHostNetwork() (bool, error) {
	store := workloadmeta.GetGlobalStore()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// wait until workloadmeta has started all collectors, to prevent
	// missing pods at startup.
	err := store.WaitForCollectors(ctx)
	if err != nil {
		return true, err
	}

	cid, err := metrics.GetProvider().GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return true, err
	}

	pod, err := store.GetKubernetesPod(cid)
	if err != nil {
		return true, err
	}

	return pod.HostNetwork, nil
}
