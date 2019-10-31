// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
)

func NewTestCommonClusterCollector(client apiserver.APICollectorClient) ClusterTopologyCollector {
	instance := topology.Instance{Type: "kubernetes", URL: "test-cluster-name"}

	return NewClusterTopologyCollector(instance, client)
}
