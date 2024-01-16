// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package clusteragent

import (
	"embed"
	"fmt"
	"io"
	"path"
	"time"

	textTemplate "text/template"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

func getLeaderElectionDetails() map[string]string {
	leaderElectionStats := make(map[string]string)

	record, err := leaderelection.GetLeaderElectionRecord()
	if err != nil {
		leaderElectionStats["status"] = "Failing"
		leaderElectionStats["error"] = err.Error()
		return leaderElectionStats
	}
	leaderElectionStats["leaderName"] = record.HolderIdentity
	leaderElectionStats["acquiredTime"] = record.AcquireTime.Format(time.RFC1123)
	leaderElectionStats["renewedTime"] = record.RenewTime.Format(time.RFC1123)
	leaderElectionStats["transitions"] = fmt.Sprintf("%d transitions", record.LeaderTransitions)
	leaderElectionStats["status"] = "Running"
	return leaderElectionStats
}

// GetDCAStatus collect the DCA agent information and return it in a map
func GetDCAStatus(stats map[string]interface{}) {
	clusterAgentDetails := make(map[string]string)

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

	stats["clusterAgentStatus"] = clusterAgentDetails
}

// Provider provides the functionality to populate the status output with the collector information
type Provider struct{}

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
func (Provider) JSON(verbose bool, stats map[string]interface{}) error {
	GetDCAStatus(stats)

	return nil
}

func (Provider) Text(_ bool, buffer io.Writer) error {
	return renderText(buffer, getStatusInfo())
}

func (Provider) HTML(_ bool, buffer io.Writer) error {
	return nil
}

func renderText(buffer io.Writer, data any) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("status_templates", "clusteragent.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("clusteragent").Funcs(status.TextFmap()).Parse(string(tmpl)))
	return t.Execute(buffer, data)
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	GetDCAStatus(stats)

	return stats
}
