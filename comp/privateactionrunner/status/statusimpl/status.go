// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package statusimpl implements the private action runner status provider.
package statusimpl

import (
	"embed"
	"io"
	"strings"

	"go.uber.org/fx"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/comp/core/config"
	statuscomp "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	parEnabledKey    = "private_action_runner.enabled"
	parSelfEnrollKey = "private_action_runner.self_enroll"
	parURNKey        = "private_action_runner.urn"
	parLogFileKey    = "private_action_runner.log_file"
)

type dependencies struct {
	fx.In

	Config config.Component
}

type provides struct {
	fx.Out

	StatusProvider statuscomp.InformationProvider
}

type statusProvider struct {
	config    config.Component
	isRunning func() (bool, error)
}

// Module defines the fx options for the private action runner status provider.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus),
	)
}

func newStatus(deps dependencies) provides {
	return provides{
		StatusProvider: statuscomp.NewInformationProvider(statusProvider{
			config:    deps.Config,
			isRunning: isPrivateActionRunnerRunning,
		}),
	}
}

//go:embed status_templates
var templatesFS embed.FS

func (s statusProvider) Name() string {
	return "Private Action Runner"
}

func (s statusProvider) Section() string {
	return "Private Action Runner"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["privateActionRunnerStatus"] = s.populateStatus()
	return stats
}

func (s statusProvider) populateStatus() map[string]interface{} {
	enabled := s.config.GetBool(parEnabledKey)
	runnerStatus := map[string]interface{}{
		"enabled":       enabled,
		"selfEnroll":    s.config.GetBool(parSelfEnrollKey),
		"urnConfigured": s.config.GetString(parURNKey) != "",
		"runnerVersion": parversion.RunnerVersion,
	}

	if logFile := s.config.GetString(parLogFileKey); logFile != "" {
		runnerStatus["logFile"] = logFile
	}

	if !enabled {
		runnerStatus["status"] = "Disabled"
		return runnerStatus
	}

	isRunning, err := s.isRunning()
	if err != nil {
		runnerStatus["status"] = "Unknown"
		runnerStatus["error"] = err.Error()
		return runnerStatus
	}

	if isRunning {
		runnerStatus["status"] = "Running"
	} else {
		runnerStatus["status"] = "Not running or unreachable"
	}

	return runnerStatus
}

func isPrivateActionRunnerRunning() (bool, error) {
	processes, err := process.Processes()
	if err != nil {
		return false, err
	}

	for _, proc := range processes {
		name, err := proc.Name()
		if err != nil {
			continue
		}
		if isPrivateActionRunnerProcess(name) {
			return true, nil
		}
	}

	return false, nil
}

func isPrivateActionRunnerProcess(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "privateactionrunner") ||
		strings.Contains(name, "datadog-private-action-runner") ||
		strings.Contains(name, "datadog-agent-action")
}

func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["privateActionRunnerStatus"] = s.populateStatus()
	return nil
}

func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return statuscomp.RenderText(templatesFS, "privateactionrunner.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}
