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
	// IP: we explicitly ignore what's behind this address (nothing to do)
	case kubeEndpointResolveIP:
	// In case of unknown value, fallback to auto
	default:
		log.Warnf("Unknown resolve value: %s for endpoint: %s/%s - fallback to auto mode", resolveMode, namespace, name)
		fallthrough
	// Auto or empty (default to auto): we try to resolve the POD behind this address
	case "":
		fallthrough
	case kubeEndpointResolveAuto:
		resolveFunc = utils.ResolveEndpointConfigAuto
	}

	return resolveFunc
}
