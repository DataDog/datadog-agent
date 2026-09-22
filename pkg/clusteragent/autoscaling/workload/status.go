// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"embed"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

//go:embed status_templates
var templatesFS embed.FS

// statusStore is the live autoscaling store, registered by the provider at
// startup. It is nil until workload autoscaling actually starts, which is how
// the status distinguishes "not started" from "started with zero autoscalers".
var statusStore struct {
	sync.RWMutex
	store      *store
	isLeader   func() bool
	rcInstance string
}

// InitStatus registers the live state used by the workload autoscaling status
// section. rcInstance names the Remote Configuration client serving the
// autoscaling products, so the status can point at the right RC instance when
// extra clients are configured.
func InitStatus(store *store, isLeader func() bool, rcInstance string) {
	statusStore.Lock()
	defer statusStore.Unlock()
	statusStore.store = store
	statusStore.isLeader = isLeader
	statusStore.rcInstance = rcInstance
}

// productStatus tracks what the last Remote Configuration update for a product
// carried. Versions are per-config, so the highest one in an update is the most
// useful single number to show.
type productStatus struct {
	LastUpdate    time.Time `json:"last_update"`
	LastVersion   uint64    `json:"last_version"`
	ConfigCount   int       `json:"config_count"`
	UpdateCount   uint64    `json:"update_count"`
	LastError     string    `json:"last_error,omitempty"`
	LastErrorTime time.Time `json:"last_error_time,omitempty"`
}

var rcTracker = struct {
	sync.RWMutex
	byProduct map[string]*productStatus
}{byProduct: map[string]*productStatus{}}

func trackedProduct(product string) *productStatus {
	if existing, found := rcTracker.byProduct[product]; found {
		return existing
	}
	created := &productStatus{}
	rcTracker.byProduct[product] = created
	return created
}

// recordRemoteConfigUpdate notes a Remote Configuration update for a product.
func recordRemoteConfigUpdate(product string, timestamp time.Time, update map[string]state.RawConfig) {
	rcTracker.Lock()
	defer rcTracker.Unlock()

	tracked := trackedProduct(product)
	tracked.LastUpdate = timestamp
	tracked.ConfigCount = len(update)
	tracked.UpdateCount++

	// An update carries the full current config set for the product, not a
	// delta, so both of these describe this update alone. Reset them rather
	// than accumulating: a stale high version or a recovered error would
	// otherwise be reported as current for the life of the process.
	tracked.LastVersion = 0
	tracked.LastError = ""
	tracked.LastErrorTime = time.Time{}

	for _, rawConfig := range update {
		if rawConfig.Metadata.Version > tracked.LastVersion {
			tracked.LastVersion = rawConfig.Metadata.Version
		}
	}
}

// recordRemoteConfigError notes a config that failed to apply.
func recordRemoteConfigError(product string, timestamp time.Time, err error) {
	rcTracker.Lock()
	defer rcTracker.Unlock()

	tracked := trackedProduct(product)
	tracked.LastError = err.Error()
	tracked.LastErrorTime = timestamp
}

// autoscalingProducts are the Remote Configuration products backing workload
// autoscaling. Cluster autoscaling is deliberately out of scope here.
var autoscalingProducts = []string{
	data.ProductContainerAutoscalingSettings,
	data.ProductContainerAutoscalingValues,
}

// Provider populates the workload autoscaling status section.
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "Workload Autoscaling"
}

// Section returns the section.
//
// Sections are rendered in alphabetical order (only "collector" is special
// cased), so "Autoscaling" places this group directly after "Autodiscovery".
// It is also the natural group for cluster autoscaling to join later.
func (Provider) Section() string {
	return "Autoscaling"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)
	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "workloadautoscaling.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "workloadautoscalingHTML.tmpl", buffer, getStatusInfo())
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	populateStatus(stats)
	return stats
}

func populateStatus(stats map[string]interface{}) {
	info := map[string]interface{}{}

	statusStore.RLock()
	liveStore, isLeader, rcInstance := statusStore.store, statusStore.isLeader, statusStore.rcInstance
	statusStore.RUnlock()

	if !pkgconfigsetup.Datadog().GetBool("autoscaling.workload.enabled") {
		info["Disabled"] = "Workload autoscaling is not enabled on the Cluster Agent"
		stats["workloadAutoscaling"] = info
		return
	}

	if liveStore == nil {
		// Enabled but the store is not registered yet: either still starting, or
		// StartWorkloadAutoscaling returned an error (which is logged, not fatal).
		info["Started"] = false
		stats["workloadAutoscaling"] = info
		return
	}

	info["Started"] = true
	info["PodAutoscalerCount"] = liveStore.Count()
	if isLeader != nil {
		info["IsLeader"] = isLeader()
	}
	if rcInstance != "" {
		info["RemoteConfigInstance"] = rcInstance
	}

	now := time.Now()
	products := make([]map[string]interface{}, 0, len(autoscalingProducts))
	connected := false

	rcTracker.RLock()
	for _, product := range autoscalingProducts {
		entry := map[string]interface{}{"Product": product}
		tracked, found := rcTracker.byProduct[product]
		if !found || tracked.LastUpdate.IsZero() {
			// Subscribed, but the backend has not sent anything yet. This is the
			// normal state for an org with no autoscalers configured.
			entry["Received"] = false
		} else {
			connected = true
			entry["Received"] = true
			entry["LastUpdate"] = tracked.LastUpdate.UTC().Format(time.RFC3339)
			entry["LastUpdateAge"] = now.Sub(tracked.LastUpdate).Truncate(time.Second).String()
			entry["LastVersion"] = tracked.LastVersion
			entry["ConfigCount"] = tracked.ConfigCount
			entry["UpdateCount"] = tracked.UpdateCount
			if tracked.LastError != "" {
				entry["LastError"] = tracked.LastError
				entry["LastErrorTime"] = tracked.LastErrorTime.UTC().Format(time.RFC3339)
			}
		}
		products = append(products, entry)
	}
	rcTracker.RUnlock()

	sort.Slice(products, func(i, j int) bool {
		return products[i]["Product"].(string) < products[j]["Product"].(string)
	})
	info["RemoteConfigProducts"] = products
	// "Connected" means at least one autoscaling product has delivered an update
	// to this process. It is subscription-level health, not TCP connectivity --
	// see the Remote Configuration section for the transport/auth state.
	info["RemoteConfigConnected"] = connected

	stats["workloadAutoscaling"] = info
}
