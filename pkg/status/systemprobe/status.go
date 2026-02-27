// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package systemprobe fetch information about the system probe
package systemprobe

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// GetStatus returns the expvar stats of the system probe
func GetStatus(stats map[string]interface{}, socketPath string) {
	client := sysprobeclient.Get(socketPath)
	url := sysprobeclient.DebugURL("/stats")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		stats["systemProbeStats"] = map[string]interface{}{"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err)}
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		stats["systemProbeStats"] = map[string]interface{}{"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err)}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		stats["systemProbeStats"] = map[string]interface{}{"Errors": fmt.Sprintf("conn request failed: url: %s, status code: %d", req.URL, resp.StatusCode)}
		return
	}
	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		stats["systemProbeStats"] = map[string]interface{}{"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err)}
		return
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal(body, &result); err != nil {
		stats["systemProbeStats"] = map[string]interface{}{"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err)}
		return
	}
	stats["systemProbeStats"] = result
}

// GetProvider if system probe is enabled returns status.Provider otherwise returns nil
func GetProvider(config sysprobeconfig.Component, rar remoteagentregistry.Component) status.Provider {
	systemProbeConfig := config.SysProbeObject()

	if systemProbeConfig.Enabled {
		return Provider{RAR: rar}
	}

	return nil
}

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct {
	RAR remoteagentregistry.Component
}

// Name returns the name
func (Provider) Name() string {
	return "System Probe"
}

// Section return the section
func (Provider) Section() string {
	return "System Probe"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)
	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "systemprobe.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	p.populateStatus(stats)
	return stats
}

func (p Provider) populateStatus(stats map[string]interface{}) {
	if p.RAR != nil {
		agentStatus, ok := p.RAR.GetStatusByFlavor("system_probe")
		if ok {
			if agentStatus.FailureReason != "" {
				stats["systemProbeStats"] = map[string]interface{}{
					"Errors": agentStatus.FailureReason,
				}
				return
			}
			// The system-probe publishes module stats under the "modules" expvar key,
			// which is the same data previously served at /debug/stats.
			if raw, ok := agentStatus.MainSection["modules"]; ok {
				var moduleStats map[string]interface{}
				if err := json.Unmarshal([]byte(raw), &moduleStats); err == nil {
					stats["systemProbeStats"] = moduleStats
					return
				}
			}
			return
		}
	}

	stats["systemProbeStats"] = map[string]interface{}{
		"Errors": "not running or unreachable",
	}
}
