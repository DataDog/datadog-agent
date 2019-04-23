// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package clusterchecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	v1 "k8s.io/client-go/listers/core/v1"
)

// newEndpointsLister return a kube endpoints lister
func newEndpointsLister() (v1.EndpointsLister, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}
	endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
	if endpointsInformer == nil {
		return nil, fmt.Errorf("cannot get endpoint informer: %s", err)
	}
	return endpointsInformer.Lister(), nil
}
