// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"embed"
	"io"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

//go:embed status_templates
var templatesFS embed.FS

// statusStore is the live state the status reports on, nil until InitStatus runs
var statusStore struct {
	sync.RWMutex
	store    *store
	isLeader func() bool
	settings productStatus
	values   productStatus
}

// productStatus is what the last Remote Configuration update for one product
// carried. Times are wall-clock, not the retriever's injected clock, because
// they are rendered as an age against time.Now()
type productStatus struct {
	lastUpdate  time.Time
	lastVersion uint64
	configCount int
	updateCount uint64
	lastError   string
}

// record notes an update. Versions are per-config, so the highest in the update
// is the most useful single number to show
func (p *productStatus) record(update map[string]state.RawConfig) {
	p.lastUpdate = time.Now()
	p.configCount = len(update)
	p.updateCount++

	// An update carries the product's whole current config set, not a delta, so
	// reset rather than accumulate: a stale high version or a recovered error
	// would otherwise be reported as current for the life of the process
	p.lastVersion = 0
	p.lastError = ""
	for _, rawConfig := range update {
		if rawConfig.Metadata.Version > p.lastVersion {
			p.lastVersion = rawConfig.Metadata.Version
		}
	}
}

func (p productStatus) info(product string) map[string]interface{} {
	info := map[string]interface{}{"Product": product, "Received": !p.lastUpdate.IsZero()}
	if p.lastUpdate.IsZero() {
		return info
	}

	info["LastVersion"] = p.lastVersion
	info["LastUpdate"] = p.lastUpdate.UTC().Format(time.RFC3339)
	info["LastUpdateAge"] = time.Since(p.lastUpdate).Truncate(time.Second).String()
	info["ConfigCount"] = p.configCount
	info["UpdateCount"] = p.updateCount
	if p.lastError != "" {
		info["LastError"] = p.lastError
	}
	return info
}

// productStatusFor routes a product to its bucket. A switch rather than a map
// keyed by product: the set is fixed at compile time, so an unknown product is a
// programming error rather than something to allocate for
func productStatusFor(product string) *productStatus {
	switch product {
	case data.ProductContainerAutoscalingSettings:
		return &statusStore.settings
	case data.ProductContainerAutoscalingValues:
		return &statusStore.values
	}
	return nil
}

// InitStatus registers the live state used by the workload autoscaling status.
// isLeader is required.
func InitStatus(store *store, isLeader func() bool) {
	statusStore.Lock()
	defer statusStore.Unlock()
	statusStore.store = store
	statusStore.isLeader = isLeader
}

// recordRemoteConfigUpdate notes a Remote Configuration update for a product
func recordRemoteConfigUpdate(product string, update map[string]state.RawConfig) {
	statusStore.Lock()
	defer statusStore.Unlock()
	if tracked := productStatusFor(product); tracked != nil {
		tracked.record(update)
	}
}

// recordRemoteConfigError notes a config that failed to apply
func recordRemoteConfigError(product string, err error) {
	statusStore.Lock()
	defer statusStore.Unlock()
	if tracked := productStatusFor(product); tracked != nil {
		tracked.lastError = err.Error()
	}
}

// Provider populates the workload autoscaling status section
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "Workload Autoscaling"
}

// Section returns the section, shared with cluster autoscaling later
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
	liveStore, isLeader := statusStore.store, statusStore.isLeader
	settings, values := statusStore.settings, statusStore.values
	statusStore.RUnlock()

	if !pkgconfigsetup.Datadog().GetBool("autoscaling.workload.enabled") {
		info["Disabled"] = "Workload autoscaling is not enabled on the Cluster Agent"
		stats["workloadAutoscaling"] = info
		return
	}

	if liveStore == nil {
		// Still starting, or StartWorkloadAutoscaling failed before InitStatus
		info["Started"] = false
		stats["workloadAutoscaling"] = info
		return
	}

	info["Started"] = true
	info["PodAutoscalerCount"] = liveStore.Count()
	info["IsLeader"] = isLeader()

	// Remote Configuration calls back only when a product's config set changes,
	// so "no update received yet" is the normal state for an organization with no
	// remote DatadogPodAutoscaler, and a steady state stops producing updates.
	// These are a record of what arrived, not a health verdict.
	info["RemoteConfigProducts"] = []map[string]interface{}{
		settings.info(data.ProductContainerAutoscalingSettings),
		values.info(data.ProductContainerAutoscalingValues),
	}

	stats["workloadAutoscaling"] = info
}
