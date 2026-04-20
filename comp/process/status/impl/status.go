// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

type dependencies struct {
	compdef.In

	Config   config.Component
	Hostname hostnameinterface.Component
}

// Provides defines the output dependencies of the status component.
type Provides struct {
	compdef.Out

	StatusProvider status.InformationProvider
}

// NewComponent creates the status component.
func NewComponent(deps dependencies) Provides {
	return Provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			config:   deps.Config,
			hostname: deps.Hostname,
		}),
	}
}

type statusProvider struct {
	testServerURL string
	config        config.Component
	hostname      hostnameinterface.Component
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s statusProvider) Name() string {
	return "Process Agent"
}

// Section return the section
func (s statusProvider) Section() string {
	return "Process Agent"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus()

	stats["processAgentStatus"] = values

	return stats
}

func (s statusProvider) populateStatus() map[string]interface{} {
	status := make(map[string]interface{})

	var url string
	if s.testServerURL != "" {
		url = s.testServerURL
	} else {

		// Get expVar server address
		// ipc_address is deprecated in favor of cmd_host, but we still need to support it
		ipcKey := "cmd_host"
		if s.config.IsSet("ipc_address") {
			log.Warn("ipc_address is deprecated, use cmd_host instead")
			ipcKey = "ipc_address"
		}
		ipcAddr, err := system.IsLocalAddress(s.config.GetString(ipcKey))
		if err != nil {
			status["error"] = fmt.Sprintf("%s: %s", ipcKey, err)
			return status
		}

		port := s.config.GetInt("process_config.expvar_port")
		if port <= 0 {
			port = 6062 // DefaultProcessExpVarPort
		}
		url = fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, port)
	}

	agentStatus, err := processStatus.GetStatus(s.config, url, s.hostname)
	if err != nil {
		status["error"] = err.Error()
		return status
	}

	bytes, err := json.Marshal(agentStatus)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	err = json.Unmarshal(bytes, &status)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	return status
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["processAgentStatus"] = values

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "processagent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
