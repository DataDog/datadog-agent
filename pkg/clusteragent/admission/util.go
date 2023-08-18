// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
)

// useNamespaceSelector returns whether we need to fallback to using namespace selector instead of object selector.
// Returns true if `namespace_selector_fallback` is enabled and k8s version is between 1.10 and 1.14 (included).
// Kubernetes 1.15+ supports object selectors.
func useNamespaceSelector(discoveryCl discovery.DiscoveryInterface) (bool, error) {
	if !config.Datadog.GetBool("admission_controller.namespace_selector_fallback") {
		return false, nil
	}

	serverVersion, err := common.KubeServerVersion(discoveryCl, 10*time.Second)
	if err != nil {
		return false, fmt.Errorf("cannot get Kubernetes version: %w", err)
	}

	log.Infof("Got Kubernetes server version, major: %q - minor: %q", serverVersion.Major, serverVersion.Minor)

	return shouldFallback(serverVersion)
}

// shouldFallback is separated from useNamespaceSelector to ease testing.
func shouldFallback(v *version.Info) (bool, error) {
	if v.Major == "1" && len(v.Minor) >= 2 {
		minor, err := strconv.Atoi(v.Minor[:2])
		if err != nil {
			return false, fmt.Errorf("cannot parse server minor version %q: %w", v.Minor[:2], err)
		}

		if minor <= 14 && minor >= 10 {
			return true, nil
		}
	}

	return false, nil
}
