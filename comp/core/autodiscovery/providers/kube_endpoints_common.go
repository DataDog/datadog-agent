// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type endpointResolveMode string

const (
	kubeEndpointResolveAuto endpointResolveMode = "auto"
	kubeEndpointResolveIP   endpointResolveMode = "ip"
)

// getEndpointResolveFunc returns a function that resolves the endpoint address
func getEndpointResolveFunc(resolveMode endpointResolveMode, namespace, name string) func(*integration.Config, v1.EndpointAddress) {
	// Check resolve annotation to know how we should process this endpoint
	var resolveFunc func(*integration.Config, v1.EndpointAddress)

	switch resolveMode {
	case kubeEndpointResolveIP:
		// IP: we explicitly ignore what's behind this address (nothing to do)

	case "", kubeEndpointResolveAuto:
		// Auto or empty (default to auto): we try to resolve the POD behind this address
		resolveFunc = utils.ResolveEndpointConfigAuto

	default:
		// Unknown value: log warning and fallback to auto mode
		log.Warnf("Unknown resolve value: %s for endpoint: %s/%s - falling back to auto mode", resolveMode, namespace, name)
		resolveFunc = utils.ResolveEndpointConfigAuto
	}

	return resolveFunc
}
