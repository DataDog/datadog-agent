// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	windowssvc "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/windows"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
	// 4) Ensure DDOT service exists/updated (no args -> service mode), then start it
	if err = ensureDDOTService(); err != nil {
		return fmt.Errorf("failed to install ddot service: %w", err)
	}
	if err = startServiceIfExists(otelServiceName); err != nil {
		return fmt.Errorf("failed to start ddot service: %w", err)
	}
	return nil
}

// preRemoveDatadogAgentDdot performs pre-removal steps for the DDOT package on Windows
// All the steps are allowed to fail
func preRemoveDatadogAgentDdot(ctx HookContext) error {
	_ = stopServiceIfExists(otelServiceName)
	_ = deleteServiceIfExists(otelServiceName)
	if !ctx.Upgrade {
		// Remove config and revert datadog.yaml changes
		_ = os.Remove(filepath.Join(paths.DatadogDataDir, "otel-config.yaml"))
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
	// Prefer packaged example from the installed package repository
	example := filepath.Join(paths.PackagesPath, agentDDOTPackage, "stable", "etc", "datadog-agent", "otel-config.yaml.example")
	// Fallback to local ProgramData example if needed
	if _, err := os.Stat(example); err != nil {
		alt := filepath.Join(paths.DatadogDataDir, "otel-config.yaml.example")
		if _, err2 := os.Stat(alt); err2 == nil {
			example = alt
		}
	}
	out := filepath.Join(paths.DatadogDataDir, "otel-config.yaml")

	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	apiKey, _ := cfg["api_key"].(string)
	site, _ := cfg["site"].(string)

	exampleData, err := os.ReadFile(example)
	if err != nil {
		return err
	}
	content := string(exampleData)
	if apiKey != "" {
		content = strings.ReplaceAll(content, "${env:DD_API_KEY}", apiKey)
	}
	if site != "" {
		content = strings.ReplaceAll(content, "${env:DD_SITE}", site)
	}
	return os.WriteFile(out, []byte(content), 0o600)
}

// enableOtelCollectorConfigWindows adds otelcollector.enabled and agent_ipc defaults to datadog.yaml
func enableOtelCollectorConfigWindows(_ context.Context) error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		return err
	}
	var existing map[string]any
	if err := yaml.Unmarshal(data, &existing); err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	existing["otelcollector"] = map[string]any{"enabled": true}
	existing["agent_ipc"] = map[string]any{"port": 5009, "config_refresh_interval": 60}
	updated, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(ddYaml, updated, 0o600)
}

// disableOtelCollectorConfigWindows removes otelcollector and agent_ipc from datadog.yaml
func disableOtelCollectorConfigWindows() error {
	ddYaml := filepath.Join(paths.DatadogDataDir, "datadog.yaml")
	data, err := os.ReadFile(ddYaml)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var existing map[string]any
	if err := yaml.Unmarshal(data, &existing); err != nil {
		return err
	}
	delete(existing, "otelcollector")
	delete(existing, "agent_ipc")
	updated, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(ddYaml, updated, 0o600)
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
		// No-op: service exists; could update config here if needed
		return nil
	}
	s, err = m.CreateService(otelServiceName, bin, mgr.Config{
		DisplayName:      "Datadog Distribution of OpenTelemetry Collector",
		Description:      "Datadog OpenTelemetry Collector",
		StartType:        mgr.StartAutomatic,
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
	_, _ = s.Control(svc.Stop)
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
