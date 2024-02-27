// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package hostname provides the hostanme information for the cluster agent
package hostname

import (
	"context"
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
)

// Provider provides the functionality to populate the status output
type Provider struct {
	config config.Component
}

//go:embed status_templates
var templatesFS embed.FS

// NewProvider returns a Provider struct
func NewProvider(conf config.Component) Provider {
	return Provider{
		config: conf,
	}
}

// Name returns the name
func (Provider) Name() string {
	return "Hostname"
}

// Index returns the index
func (Provider) Index() int {
	return 1
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats, p.config)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "hostname.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats, p.config)

	return stats
}

func populateStatus(stats map[string]interface{}, config config.Component) {
	hostnameStatsJSON := []byte(expvar.Get("hostname").String())
	hostnameStats := make(map[string]interface{})
	json.Unmarshal(hostnameStatsJSON, &hostnameStats) //nolint:errcheck
	stats["hostnameStats"] = hostnameStats

	hostMetadata := hostMetadataUtils.GetFromCache(context.TODO(), config)
	metadataStats := make(map[string]interface{})
	hostMetadataBytes, _ := json.Marshal(hostMetadata)
	json.Unmarshal(hostMetadataBytes, &metadataStats) //nolint:errcheck

	stats["metadata"] = metadataStats
}
