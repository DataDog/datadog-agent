// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agent contains a process-agent component
package agent

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
)

// StatusProvider is the type for process component status methods
type StatusProvider struct {
	testServerURL string
	config        config.Component
}

// NewStatusProvider fetches the status
func NewStatusProvider(Config config.Component) *StatusProvider {
	return &StatusProvider{
		config: Config,
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s StatusProvider) Name() string {
	return "Process Component"
}

// Section return the section
func (s StatusProvider) Section() string {
	return "Process Component"
}

func (s StatusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus()

	stats["processComponentStatus"] = values

	return stats
}

func (s StatusProvider) populateStatus() map[string]interface{} {
	status := make(map[string]interface{})

	var url string
	if s.testServerURL != "" {
		url = s.testServerURL
	} else {

		// Get expVar server address
		ipcAddr, err := ddconfig.GetIPCAddress()
		if err != nil {
			status["error"] = fmt.Sprintf("%v", err.Error())
			return status
		}

		port := s.config.GetInt("expvar_port")
		url = fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, port)
	}

	agentStatus, err := processStatus.GetStatus(s.config, url)
	if err != nil {
		status["error"] = fmt.Sprintf("%v", err.Error())
		return status
	}

	bytes, err := json.Marshal(agentStatus)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("%v", err.Error()),
		}
	}

	err = json.Unmarshal(bytes, &status)
	if err != nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("%v", err.Error()),
		}
	}

	return status
}

// JSON populates the status map
func (s StatusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["processComponentStatus"] = values

	return nil
}

// Text renders the text output
func (s StatusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "processcomponent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s StatusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
