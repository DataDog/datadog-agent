// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configure shares host Agent rendering, private file delivery and initialization.
package configure

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	compout "github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/agentconfig"
	"regexp"
)

func ValidateIntegrations(integrations map[string]string) error {
	for folder := range integrations {
		if !integrationFolderPattern.MatchString(folder) || folder == "." || folder == ".." {
			return fmt.Errorf("invalid integration folder %q", folder)
		}
	}
	return nil
}

func Apply(env *environments.Host, config string, integrations map[string]string) error {
	return ApplyBeforeRestart(env, config, integrations, nil)
}

// ApplyBeforeRestart allows package installs to keep services masked until all
// private configuration is installed. Script installs retain their prior flow.
func ApplyBeforeRestart(env *environments.Host, config string, integrations map[string]string, beforeRestart func() error) error {
	if err := ValidateIntegrations(integrations); err != nil {
		return err
	}
	if err := writeRemoteFile(env, "/etc/datadog-agent/datadog.yaml", config); err != nil {
		return err
	}
	for folder, content := range integrations {
		configDir := "/etc/datadog-agent/conf.d/" + folder
		if _, err := env.RemoteHost.Execute("sudo mkdir -p " + configDir); err != nil {
			return fmt.Errorf("creating integration directory %s: %w", configDir, err)
		}
		if err := writeRemoteFile(env, configDir+"/conf.yaml", content); err != nil {
			return err
		}
	}
	if beforeRestart != nil {
		if err := beforeRestart(); err != nil {
			return err
		}
	}
	if _, err := env.RemoteHost.Execute("sudo systemctl restart datadog-agent"); err != nil {
		return fmt.Errorf("restarting Agent: %w", err)
	}

	if env.Agent == nil {
		env.Agent = &components.RemoteHostAgent{}
	}
	env.Agent.HostAgentOutput = compout.HostAgentOutput{Host: env.RemoteHost.HostOutput}
	if err := env.Agent.InitFromHost(env.RemoteHost); err != nil {
		return fmt.Errorf("initializing installed Agent: %w", err)
	}
	return nil
}

var integrationFolderPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// buildAgentConfig delegates to the shared agentconfig policy so the install
// script (on a provisioned VM) and the binary installer (in a container) wire
// fakeintake identically.
func Generate(env *environments.Host, apiKey, extraConfig string) (string, error) {
	var endpoint *agentconfig.Endpoint
	if env.FakeIntake != nil {
		endpoint = &agentconfig.Endpoint{
			Scheme: env.FakeIntake.Scheme,
			Host:   env.FakeIntake.Host,
			Port:   int(env.FakeIntake.Port),
		}
	}
	return agentconfig.Generate(apiKey, endpoint, extraConfig)
}

func writeRemoteFile(env *environments.Host, filePath, content string) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	stage := "/tmp/e2ectl-" + hex.EncodeToString(nonce[:]) + ".yaml"
	if err := env.RemoteHost.WritePrivateFile(stage, []byte(content)); err != nil {
		return fmt.Errorf("staging private Agent file: %w", err)
	}
	defer env.RemoteHost.Execute("rm -f " + stage) // paths are installer-owned, never content
	if _, err := env.RemoteHost.Execute("sudo install -o dd-agent -g dd-agent -m 0600 " + stage + " " + filePath); err != nil {
		return fmt.Errorf("installing private Agent file: %w", err)
	}
	return nil
}
