// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	layerKeys = "fleet_layers"

	configDatadogYAML       = "datadog.yaml"
	configSecurityAgentYAML = "security-agent.yaml"
	configSystemProbeYAML   = "system-probe.yaml"
)

// agentConfig represents the agent configuration from the CDN.
type agentConfig struct {
	version   string
	policyIDs []string

	datadog       []byte
	securityAgent []byte
	systemProbe   []byte
}

// agentConfigLayer is a config layer that can be merged with other layers into a config.
type agentConfigLayer struct {
	ID                  string                 `json:"name"`
	AgentConfig         map[string]interface{} `json:"config"`
	SecurityAgentConfig map[string]interface{} `json:"security_agent"`
	SystemProbeConfig   map[string]interface{} `json:"system_probe"`
}

// State returns the agent policies state
func (a *agentConfig) State() *pbgo.PoliciesState {
	return &pbgo.PoliciesState{
		MatchedPolicies: a.policyIDs,
		Version:         a.version,
	}
}

func newAgentConfig(orderedLayers ...[]byte) (*agentConfig, error) {
	// Compile ordered layers into a single config
	policyIDs := []string{}
	compiledLayer := &agentConfigLayer{
		AgentConfig:         map[string]interface{}{},
		SecurityAgentConfig: map[string]interface{}{},
		SystemProbeConfig:   map[string]interface{}{},
	}
	for _, rawLayer := range orderedLayers {
		layer := &agentConfigLayer{}
		if err := json.Unmarshal(rawLayer, layer); err != nil {
			log.Warnf("Failed to unmarshal layer: %v", err)
			continue
		}
		if layer.AgentConfig == nil && layer.SecurityAgentConfig == nil && layer.SystemProbeConfig == nil {
			// Only add layers that have at least one config that matches the agent
			continue
		}

		policyIDs = append(policyIDs, layer.ID)

		if layer.AgentConfig != nil {
			agentConfig, err := merge(compiledLayer.AgentConfig, layer.AgentConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.AgentConfig = agentConfig.(map[string]interface{})
		}

		if layer.SecurityAgentConfig != nil {
			securityAgentConfig, err := merge(compiledLayer.SecurityAgentConfig, layer.SecurityAgentConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.SecurityAgentConfig = securityAgentConfig.(map[string]interface{})
		}

		if layer.SystemProbeConfig != nil {
			systemProbeAgentConfig, err := merge(compiledLayer.SystemProbeConfig, layer.SystemProbeConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.SystemProbeConfig = systemProbeAgentConfig.(map[string]interface{})
		}
	}

	// Report applied layers
	compiledLayer.AgentConfig[layerKeys] = policyIDs

	// Marshal into YAML configs
	config, err := marshalYAMLConfig(compiledLayer.AgentConfig)
	if err != nil {
		return nil, err
	}
	securityAgentConfig, err := marshalYAMLConfig(compiledLayer.SecurityAgentConfig)
	if err != nil {
		return nil, err
	}
	systemProbeConfig, err := marshalYAMLConfig(compiledLayer.SystemProbeConfig)
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	version, err := json.Marshal(compiledLayer)
	if err != nil {
		return nil, err
	}
	hash.Write(version)

	return &agentConfig{
		version:   fmt.Sprintf("%x", hash.Sum(nil)),
		policyIDs: policyIDs,

		datadog:       config,
		securityAgent: securityAgentConfig,
		systemProbe:   systemProbeConfig,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (a *agentConfig) Write(dir string) error {
	ddAgentUID, ddAgentGID, err := getAgentIDs()
	if err != nil {
		return fmt.Errorf("error getting dd-agent user and group IDs: %w", err)
	}

	if a.datadog != nil {
		err = os.WriteFile(filepath.Join(dir, configDatadogYAML), []byte(a.datadog), 0640)
		if err != nil {
			return fmt.Errorf("could not write %s: %w", configDatadogYAML, err)
		}
		if runtime.GOOS != "windows" {
			err = os.Chown(filepath.Join(dir, configDatadogYAML), ddAgentUID, ddAgentGID)
			if err != nil {
				return fmt.Errorf("could not chown %s: %w", configDatadogYAML, err)
			}
		}
	}
	if a.securityAgent != nil {
		err = os.WriteFile(filepath.Join(dir, configSecurityAgentYAML), []byte(a.securityAgent), 0440)
		if err != nil {
			return fmt.Errorf("could not write %s: %w", configSecurityAgentYAML, err)
		}
		if runtime.GOOS != "windows" {
			err = os.Chown(filepath.Join(dir, configSecurityAgentYAML), 0, ddAgentGID) // root:dd-agent
			if err != nil {
				return fmt.Errorf("could not chown %s: %w", configSecurityAgentYAML, err)
			}
		}
	}
	if a.systemProbe != nil {
		err = os.WriteFile(filepath.Join(dir, configSystemProbeYAML), []byte(a.systemProbe), 0440)
		if err != nil {
			return fmt.Errorf("could not write %s: %w", configSecurityAgentYAML, err)
		}
		if runtime.GOOS != "windows" {
			err = os.Chown(filepath.Join(dir, configSystemProbeYAML), 0, ddAgentGID) // root:dd-agent
			if err != nil {
				return fmt.Errorf("could not chown %s: %w", configSecurityAgentYAML, err)
			}
		}
	}
	return writePolicyMetadata(a, dir)
}

// getAgentIDs returns the UID and GID of the dd-agent user and group.
func getAgentIDs() (uid, gid int, err error) {
	ddAgentUser, err := user.Lookup("dd-agent")
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent user not found: %w", err)
	}
	ddAgentGroup, err := user.LookupGroup("dd-agent")
	if err != nil {
		return -1, -1, fmt.Errorf("dd-agent group not found: %w", err)
	}
	ddAgentUID, err := strconv.Atoi(ddAgentUser.Uid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent UID to int: %w", err)
	}
	ddAgentGID, err := strconv.Atoi(ddAgentGroup.Gid)
	if err != nil {
		return -1, -1, fmt.Errorf("error converting dd-agent GID to int: %w", err)
	}
	return ddAgentUID, ddAgentGID, nil
}
