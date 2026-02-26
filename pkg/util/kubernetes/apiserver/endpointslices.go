// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/mod/semver"
)

const endpointSlicesCacheKey = "useEndpointSlices"

// UseEndpointSlices returns true if the Agent config has enabled endpoint slices and the running
// Kubernetes server version supports it. EndpointSlices became generally available starting from
// Kubernetes version 1.21+. The Agent should fall back to the endpoints API if the config
// is enabled but the server version does not support it.
func UseEndpointSlices() bool {
	if use, found := cache.Cache.Get(endpointSlicesCacheKey); found {
		return use.(bool)
	}

	if !pkgconfigsetup.Datadog().GetBool("kubernetes_use_endpoint_slices") {
		log.Debug("kubernetes_use_endpoint_slices is disabled, using deprecated endpoints API.")
		cache.Cache.Set(endpointSlicesCacheKey, false, time.Hour)
		return false
	}

	apiserverClient, err := GetAPIClient()
	if err != nil {
		log.Warnf("Couldn't get apiserver client, cannot check if kubernetes version supports endpoint slices.")
		// Not caching because we don't want to cache the result of a failure.
		return true
	}

	serverVersion, err := common.KubeServerVersion(apiserverClient.Cl.Discovery(), 10*time.Second)
	if err != nil {
		log.Warnf("Couldn't get apiserver version, cannot check if kubernetes version supports endpoint slices.")
		// Fail open because the config was enabled, even though we can't check the version.
		// Not caching because we don't want to cache the result of a failure.
		return true
	}

	if semver.IsValid(serverVersion.String()) && semver.Compare(serverVersion.String(), "v1.21.0") >= 0 {
		cache.Cache.Set(endpointSlicesCacheKey, true, time.Hour)
		return true
	}
	log.Warnf("Endpoint Slices not supported in Kubernetes version %s. Falling back to core/v1/Endpoints.", serverVersion.String())
	cache.Cache.Set(endpointSlicesCacheKey, false, time.Hour)
	return false
}
