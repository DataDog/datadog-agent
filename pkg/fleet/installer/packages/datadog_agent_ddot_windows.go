// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	windowssvc "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/windows"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

var datadogAgentDDOTPackage = hooks{
	preInstall:  preInstallDatadogAgentDDOT,
	postInstall: postInstallDatadogAgentDdot,
	preRemove:   preRemoveDatadogAgentDdot,
}

const (
	agentDDOTPackage = "datadog-agent-ddot"
	otelServiceName  = "datadog-otel-agent"
	coreAgentService = "datadogagent"
)

// preInstallDatadogAgentDDOT performs pre-installation steps for DDOT on Windows
func preInstallDatadogAgentDDOT(_ HookContext) error {
	// Best effort stop and delete existing service
	_ = stopServiceIfExists(otelServiceName)
	_ = deleteServiceIfExists(otelServiceName)
	return nil
}

// postInstallDatadogAgentDdot performs post-installation steps for the DDOT package on Windows
func postInstallDatadogAgentDdot(ctx HookContext) (err error) {
	// 1) Write otel-config.yaml with API key/site substitutions
	if err = writeOTelConfigWindows(); err != nil {
		return fmt.Errorf("could not write otel-config.yaml: %w", err)
	}
	// 2) Enable otelcollector in datadog.yaml
	if err = enableOtelCollectorConfigWindows(ctx.Context); err != nil {
		return fmt.Errorf("failed to enable otelcollector: %w", err)
	}
	// 3) Restart main Agent services to pick up config changes
	if err = windowssvc.NewWinServiceManager().RestartAgentServices(ctx.Context); err != nil {
		return fmt.Errorf("failed to restart agent services: %w", err)
	}
	// 4) Ensure DDOT service exists/updated, then start it (best-effort)
	if err = ensureDDOTService(); err != nil {
		return fmt.Errorf("failed to install ddot service: %w", err)
	}
	// Start DDOT only when core Agent is running (handle StartPending) and credentials exist
	running, _ := winutil.IsServiceRunning(coreAgentService)
	if !running {
		// If core Agent is still starting, wait briefly for it to leave StartPending
		ctxCA, cancelCA := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCA()
		if st, err := winutil.WaitForPendingStateChange(ctxCA, coreAgentService, svc.StartPending); err != nil || st != svc.Running {
			log.Warnf("DDOT: skipping service start (core Agent not running; state=%d, err=%v)", st, err)
			return nil
		}
	}
	if ak := readAPIKeyFromDatadogYAML(); ak == "" {
		log.Warnf("DDOT: skipping service start (no API key configured)")
		return nil
	}
	if err = startServiceIfExists(otelServiceName); err != nil {
		log.Warnf("DDOT: failed to start service: %v", err)
		return nil
	}
	// Fail fast if the service exits or transitions away from StartPending
	ctxWait, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, err := winutil.WaitForPendingStateChange(ctxWait, otelServiceName, svc.StartPending)
	if err != nil {
		log.Warnf("DDOT: service %q did not reach Running state: %s", otelServiceName, err)
		return nil
	}
	if state != svc.Running {
		log.Warnf("DDOT: service %q transitioned to state %d instead of Running", otelServiceName, state)
		return nil
	}
	return nil
}

// waitForServiceRunning waits until the given Windows service reaches the Running state or times out
// (removed) waitForServiceRunning and isServiceRunning helpers were replaced by
// winutil.WaitForPendingStateChange and winutil.IsServiceRunning

// readAPIKeyFromDatadogYAML reads the api_key from ProgramData datadog.yaml, returns empty string if unset/unknown
func readAPIKeyFromDatadogYAML() string {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return ""
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	if v, ok := cfg["api_key"].(string); ok {
		return v
	}
	return ""
}

// preRemoveDatadogAgentDdot performs pre-removal steps for the DDOT package on Windows
// All the steps are allowed to fail
func preRemoveDatadogAgentDdot(ctx HookContext) error {
	_ = stopServiceIfExists(otelServiceName)
	_ = deleteServiceIfExists(otelServiceName)

	if !ctx.Upgrade {
		// Preserve otel-config.yaml; only disable the feature in datadog.yaml
		if err := disableOtelCollectorConfigWindows(); err != nil {
			log.Warnf("failed to disable otelcollector in datadog.yaml: %s", err)
		}
		// Restart core agent to pick up reverted config
		if err := windowssvc.NewWinServiceManager().RestartAgentServices(ctx.Context); err != nil {
			log.Warnf("failed to restart agent services: %s", err)
		}
	}
	return nil
}

// writeOTelConfigWindows creates otel-config.yaml by substituting API key and site values from datadog.yaml
func writeOTelConfigWindows() error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	// Prefer packaged example/template from the installed package repository
	cfgTemplate := filepath.Join(paths.PackagesPath, agentDDOTPackage, "stable", "etc", "datadog-agent", "otel-config.yaml.example")
	// Fallback to local ProgramData example/template if needed
	if _, err := os.Stat(cfgTemplate); err != nil {
		alt := filepath.Join(paths.DatadogDataDir, "otel-config.yaml.example")
		if _, err2 := os.Stat(alt); err2 == nil {
			cfgTemplate = alt
		}
	}
	out := filepath.Join(paths.DatadogDataDir, "otel-config.yaml")
	return writeOTelConfigCommon(ddYaml, cfgTemplate, out, true, 0o600)
}

// enableOtelCollectorConfigWindows adds otelcollector.enabled and agent_ipc defaults to datadog.yaml
func enableOtelCollectorConfigWindows(_ context.Context) error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	return enableOtelCollectorConfigCommon(ddYaml)
}

// disableOtelCollectorConfigWindows removes otelcollector and agent_ipc from datadog.yaml
func disableOtelCollectorConfigWindows() error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	return disableOtelCollectorConfigCommon(ddYaml)
}

// ensureDDOTService ensures the DDOT service exists and is configured correctly
func ensureDDOTService() error {
	bin := filepath.Join(paths.PackagesPath, agentDDOTPackage, "stable", "embedded", "bin", "otel-agent.exe")
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(otelServiceName)
	if err == nil {
		defer s.Close()
		// update existing, remove SCM dependency if any
		cfg, errC := s.Config()
		if errC != nil {
			return errC
		}
		changed := false
		if cfg.StartType != mgr.StartManual {
			cfg.StartType = mgr.StartManual
			changed = true
		}
		if len(cfg.Dependencies) > 0 {
			// drop SCM dependency
			cfg.Dependencies = nil
			changed = true
		}
		if changed {
			if errU := s.UpdateConfig(cfg); errU != nil {
				return errU
			}
		}
		return nil
	}
	// TODO(WINA-1619): Change service user to ddagentuser
	s, err = m.CreateService(otelServiceName, bin, mgr.Config{
		DisplayName:      "Datadog Distribution of OpenTelemetry Collector",
		Description:      "Datadog OpenTelemetry Collector",
		StartType:        mgr.StartManual,
		ServiceStartName: "", // LocalSystem
	})
	if err != nil {
		return err
	}
	defer s.Close()
	return nil
}

// stopServiceIfExists stops the service if it exists
func stopServiceIfExists(name string) error {
	// Use robust stop; ignore 'service does not exist'
	if err := winutil.StopService(name); err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	return nil
}

// startServiceIfExists starts the service if it exists
func startServiceIfExists(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer s.Close()
	return s.Start()
}

// deleteServiceIfExists deletes the service if it exists
func deleteServiceIfExists(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return nil
	}
	defer s.Close()
	return s.Delete()
}
