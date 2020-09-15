// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver
// +build !kubelet

// This provider is only useful for the cluster-agent, that does
// not have kubelet compiled it. Disable it for the node-agent
// that already has the kubelet provider available.

package hostname

import (
	"github.com/DataDog/datadog-agent/pkg/util/hostname/apiserver"
)

func init() {
	RegisterHostnameProvider("kube_apiserver", apiserver.HostnameProvider)
}
