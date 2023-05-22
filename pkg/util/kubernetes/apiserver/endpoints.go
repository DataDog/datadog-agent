// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"

	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
)

const kubeEndpointIDPrefix = "kube_endpoint_uid://"

// SearchTargetPerName returns the endpoint matching a given target name. It allows
// to retrieve a given pod's endpoint address from a service.
func SearchTargetPerName(endpoints *v1.Endpoints, targetName string) (v1.EndpointAddress, error) {
	if endpoints == nil {
		return v1.EndpointAddress{}, errors.New("nil endpoints object passed")
	}
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef == nil {
				continue
			}
			if addr.TargetRef.Name == targetName {
				return addr, nil
			}
		}
	}
	return v1.EndpointAddress{}, dderrors.NewNotFound("target named " + targetName)
}

// EntityForEndpoints builds entity strings for Endpoints
func EntityForEndpoints(namespace, name, ip string) string {
	return fmt.Sprintf("%s%s/%s/%s", kubeEndpointIDPrefix, namespace, name, ip)
}
