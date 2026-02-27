// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"runtime"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type dependencies struct {
	fx.In

	Config   config.Component
	Hostname hostnameinterface.Component
	RAR      remoteagentregistry.Component `optional:"true"`
}

type provides struct {
	fx.Out

	StatusProvider status.InformationProvider
}

// Module defines the fx options for the status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus))
}

type statusProvider struct {
	config   config.Component
	hostname hostnameinterface.Component
	rar      remoteagentregistry.Component
}

func newStatus(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			config:   deps.Config,
			hostname: deps.Hostname,
			rar:      deps.RAR,
		}),
	}
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
	result := make(map[string]interface{})

	if s.rar != nil {
		agentStatus, ok := s.rar.GetStatusByFlavor("process_agent")
		if ok {
			if agentStatus.FailureReason != "" {
				result["error"] = agentStatus.FailureReason
				return result
			}

			var expvarsMap processStatus.ExpvarsMap
			if raw, ok := agentStatus.MainSection["process_agent"]; ok {
				_ = json.Unmarshal([]byte(raw), &expvarsMap)
			}

			core := processStatus.CoreStatus{
				AgentVersion: version.AgentVersion,
				GoVersion:    runtime.Version(),
				Arch:         runtime.GOARCH,
				Config:       processStatus.ConfigStatus{LogLevel: s.config.GetString("log_level")},
				Metadata:     *hostMetadataUtils.GetFromCache(context.Background(), s.config, s.hostname),
			}
			st := processStatus.Status{
				Date:    float64(time.Now().UnixNano()),
				Core:    core,
				Expvars: processStatus.ProcessExpvars{ExpvarsMap: expvarsMap},
			}

			bytes, err := json.Marshal(st)
			if err != nil {
				result["error"] = err.Error()
				return result
			}
			if err := json.Unmarshal(bytes, &result); err != nil {
				result["error"] = err.Error()
			}
			return result
		}
	}

	result["error"] = "not running or unreachable"
	return result
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
