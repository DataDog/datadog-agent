// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"context"
	"strconv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// CSI driver ConfigMap constants (must match the CSI driver implementation)
	csiConfigMapName       = "datadog-csi-driver-config"
	csiConfigKeyVersion    = "version"
	csiConfigKeySSIEnabled = "ssi_enabled"

	// Cache TTL to avoid querying the API server too frequently
	csiConfigCacheTTL = 30 * time.Second
)

// CSIDriverStatus represents the CSI driver status read from its ConfigMap
type CSIDriverStatus struct {
	// Available is true if the CSI driver ConfigMap was found
	Available bool
	// Version is the CSI driver version
	Version string
	// SSIEnabled is true if SSI/APM injection is enabled in the CSI driver
	SSIEnabled bool
}

// csiConfigCache caches the CSI driver status to avoid frequent API calls
var (
	csiConfigCache     CSIDriverStatus
	csiConfigCacheTime time.Time
	csiConfigCacheMu   sync.RWMutex

	// csiStatusFetcher can be overridden for testing
	csiStatusFetcher = fetchCSIDriverStatus
)

// GetCSIDriverStatus returns the CSI driver status by reading its ConfigMap.
// The ConfigMap is expected to be in the same namespace as the cluster-agent.
// Results are cached for csiConfigCacheTTL to reduce API server load.
func GetCSIDriverStatus(ctx context.Context) CSIDriverStatus {
	// Check cache first
	csiConfigCacheMu.RLock()
	if time.Since(csiConfigCacheTime) < csiConfigCacheTTL {
		status := csiConfigCache
		csiConfigCacheMu.RUnlock()
		return status
	}
	csiConfigCacheMu.RUnlock()

	// Cache expired, fetch fresh data
	status := csiStatusFetcher(ctx)

	// Update cache
	csiConfigCacheMu.Lock()
	csiConfigCache = status
	csiConfigCacheTime = time.Now()
	csiConfigCacheMu.Unlock()

	return status
}

// fetchCSIDriverStatus fetches the CSI driver status from the API server.
// It assumes the CSI driver ConfigMap is in the same namespace as the cluster-agent.
func fetchCSIDriverStatus(ctx context.Context) CSIDriverStatus {
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		log.Debugf("Failed to get API client for CSI driver status: %v", err)
		return CSIDriverStatus{Available: false}
	}

	ns := namespace.GetMyNamespace()
	cm, err := apiClient.Cl.CoreV1().ConfigMaps(ns).Get(ctx, csiConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Debugf("CSI driver ConfigMap %s/%s not found", ns, csiConfigMapName)
		} else {
			log.Debugf("Failed to get CSI driver ConfigMap %s/%s: %v", ns, csiConfigMapName, err)
		}
		return CSIDriverStatus{Available: false}
	}

	ssiEnabled, _ := strconv.ParseBool(cm.Data[csiConfigKeySSIEnabled])

	status := CSIDriverStatus{
		Available:  true,
		Version:    cm.Data[csiConfigKeyVersion],
		SSIEnabled: ssiEnabled,
	}

	log.Debugf("CSI driver status: version=%s, ssi_enabled=%v (from %s/%s)",
		status.Version, status.SSIEnabled, ns, csiConfigMapName)

	return status
}

// InvalidateCSIConfigCache invalidates the CSI driver config cache.
// This is useful for testing or when the CSI driver configuration changes.
func InvalidateCSIConfigCache() {
	csiConfigCacheMu.Lock()
	csiConfigCacheTime = time.Time{}
	csiConfigCacheMu.Unlock()
}

// SetCSIStatusFetcherForTest allows tests to override the CSI status fetcher.
// It returns a cleanup function that restores the original fetcher.
func SetCSIStatusFetcherForTest(fetcher func(ctx context.Context) CSIDriverStatus) func() {
	csiConfigCacheMu.Lock()
	originalFetcher := csiStatusFetcher
	csiStatusFetcher = fetcher
	csiConfigCacheTime = time.Time{} // Invalidate cache
	csiConfigCacheMu.Unlock()

	return func() {
		csiConfigCacheMu.Lock()
		csiStatusFetcher = originalFetcher
		csiConfigCacheTime = time.Time{} // Invalidate cache
		csiConfigCacheMu.Unlock()
	}
}
