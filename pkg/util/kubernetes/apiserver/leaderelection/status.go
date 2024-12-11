// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package leaderelection

import (
	"embed"
	"fmt"
	"io"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

// Provider provides the functionality to populate the status output
type Provider struct{}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Leader Election"
}

// Section return the section
func (Provider) Section() string {
	return "Leader Election"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "leaderelection.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func populateStatus(stats map[string]interface{}) {
	leaderElectionStats := make(map[string]string)
	stats["leaderelection"] = leaderElectionStats

	record, err := GetLeaderElectionRecord()
	if err != nil {
		leaderElectionStats["status"] = "Failing"
		leaderElectionStats["error"] = err.Error()
		return
	}
	leaderElectionStats["leaderName"] = record.HolderIdentity
	leaderElectionStats["acquiredTime"] = record.AcquireTime.Format(time.RFC1123)
	leaderElectionStats["renewedTime"] = record.RenewTime.Format(time.RFC1123)
	leaderElectionStats["transitions"] = fmt.Sprintf("%d transitions", record.LeaderTransitions)
	leaderElectionStats["status"] = "Running"
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats)

	return stats
}
