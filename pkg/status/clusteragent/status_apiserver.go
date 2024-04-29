// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package clusteragent fetch information about the cluster agent
package clusteragent

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
)

// GetDCAStatus collect the DCA agent information and return it in a map
func GetDCAStatus(stats map[string]interface{}) {
	clusterAgentDetails := make(map[string]string)
	stats["clusterAgentStatus"] = clusterAgentDetails

	dcaCl, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		clusterAgentDetails["DetectionError"] = err.Error()
		return
	}
	clusterAgentDetails["Endpoint"] = dcaCl.ClusterAgentAPIEndpoint()

	ver, err := dcaCl.GetVersion()
	if err != nil {
		clusterAgentDetails["ConnectionError"] = err.Error()
		return
	}
	clusterAgentDetails["Version"] = ver.String()
}

// Provider provides the functionality to populate the status output
type Provider struct{}

// GetProvider if cluster agent is enabled returns status.Provider otherwise returns nil
func GetProvider(conf config.Component) status.Provider {
	if conf.GetBool("cluster_agent.enabled") || conf.GetBool("cluster_checks.enabled") {
		return Provider{}
	}

	return nil
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Datadog Cluster Agent"
}

// Section return the section
func (Provider) Section() string {
	return "Datadog Cluster Agent"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	GetDCAStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "clusteragent.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	GetDCAStatus(stats)

	return stats
}
