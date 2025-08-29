// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package clustercheckimpl

import (
	"embed"
	"io"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Provider provides the functionality to populate the status output
type Provider struct {
	ClusterChecksHandler option.Option[clusterchecks.Component]
}

// NewProvider creates a new Provider with optional cluster checks handler
func NewProvider(clusterChecksHandler option.Option[clusterchecks.Component]) Provider {
	return Provider{
		ClusterChecksHandler: clusterChecksHandler,
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (p Provider) Name() string {
	return "Cluster Checks Dispatching"
}

// Section return the section
func (p Provider) Section() string {
	return "Cluster Checks Dispatching"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "clusterchecks.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func (p Provider) populateStatus(stats map[string]interface{}) {
	if pkgconfigsetup.Datadog().GetBool("cluster_checks.enabled") {
		if handler, ok := p.ClusterChecksHandler.Get(); ok {
			if cchecks, err := handler.GetStats(); err == nil {
				stats["clusterchecks"] = cchecks
			}
		}
	}
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}
