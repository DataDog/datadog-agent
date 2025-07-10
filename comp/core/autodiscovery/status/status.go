// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status fetch information needed to render the 'autodiscovery' section of the status page.
package status

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/status"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// populateStatus populates the status stats
func populateStatus(ac autodiscovery.Component, wf workloadfilter.Component, stats map[string]interface{}) {
	stats["adEnabledFeatures"] = env.GetDetectedFeatures()
	stats["adConfigErrors"] = ac.GetAutodiscoveryErrors()
	stats["filterErrors"] = getAutodiscoveryFilterErrors(wf)
}

// GetAutodiscoveryFilterErrors returns a map of all autodiscovery filter errors
func getAutodiscoveryFilterErrors(wf workloadfilter.Component) map[string]struct{} {
	var workloadfilterList []workloadfilter.ContainerFilter
	for _, filterScope := range []workloadfilter.Scope{workloadfilter.MetricsFilter, workloadfilter.LogsFilter, workloadfilter.GlobalFilter} {
		workloadfilterList = append(workloadfilterList, workloadfilter.FlattenFilterSets(workloadfilter.GetAutodiscoveryFilters(filterScope))...)
	}
	workloadfilterErrors := wf.GetContainerFilterInitializationErrors(workloadfilterList)

	filterErrorsSet := make(map[string]struct{})
	for _, err := range workloadfilterErrors {
		filterErrorsSet[err.Error()] = struct{}{}
	}
	return filterErrorsSet
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct {
	ac          autodiscovery.Component
	filterStore workloadfilter.Component
}

// GetProvider if agent is running in a container environment returns status.Provider otherwise returns nil
func GetProvider(acComp autodiscovery.Component, filterStore workloadfilter.Component) status.Provider {
	if env.IsContainerized() {
		return Provider{ac: acComp, filterStore: filterStore}
	}

	return nil
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(p.ac, p.filterStore, stats)

	return stats
}

// Name returns the name
func (p Provider) Name() string {
	return "Autodiscovery"
}

// Section return the section
func (p Provider) Section() string {
	return "Autodiscovery"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(p.ac, p.filterStore, stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "autodiscovery.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}
