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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

//go:embed status_templates
var templatesFS embed.FS

// statusStore is the live state the status reports on, nil until InitStatus runs
var statusStore struct {
	sync.RWMutex
	store    *store
	isLeader func() bool
	// Wall-clock, not the retriever's injected clock: only ever rendered as an
	// age against time.Now(), so both ends must come from the same clock
	lastRemoteConfigUpdate time.Time
}

// InitStatus registers the live state used by the workload autoscaling status.
// isLeader is required.
func InitStatus(store *store, isLeader func() bool) {
	statusStore.Lock()
	defer statusStore.Unlock()
	statusStore.store = store
	statusStore.isLeader = isLeader
}

// recordRemoteConfigUpdate notes that a Remote Configuration update arrived
func recordRemoteConfigUpdate() {
	statusStore.Lock()
	defer statusStore.Unlock()
	statusStore.lastRemoteConfigUpdate = time.Now()
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
	liveStore, isLeader, lastUpdate := statusStore.store, statusStore.isLeader, statusStore.lastRemoteConfigUpdate
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

	// Not a health signal either way: Remote Configuration calls back only when
	// the config set changes, so an org with no remote DatadogPodAutoscaler never
	// gets one and stable configs stop producing them. Report the fact only.
	info["RemoteConfigReceived"] = !lastUpdate.IsZero()
	if !lastUpdate.IsZero() {
		info["LastRemoteConfigUpdateAge"] = time.Since(lastUpdate).Truncate(time.Second).String()
	}

	stats["workloadAutoscaling"] = info
}
