// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rcclient

import (
	"encoding/json"
	"os"

	yamlv2 "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var apmTracingFilePath = "/opt/datadog-agent/run/inject_config.yaml"

// InvalidAPMTracingPayload indicates we received an APM_TRACING payload we were unable to decode
const InvalidAPMTracingPayload = "INVALID_APM_TRACING_PAYLOAD"

// MissingServiceTarget indicates we were missing the service_target field
const MissingServiceTarget = "MISSING_SERVICE_TARGET"

// FileWriteFailure indicates we were unable to write the RC Updates to a local file for use by the injector
const FileWriteFailure = "FILE_WRITE_FAILURE"

// DuplicateHostConfig indicates received more than one InfraTarget configuration with a different env,
// this leads to inconsistent env values
const DuplicateHostConfig = "DUPLICATE_HOST_CONFIG"

type serviceEnvConfig struct {
	Service        string `yaml:"service"`
	Env            string `yaml:"env"`
	TracingEnabled bool   `yaml:"tracing_enabled"`
}

type tracingEnabledConfig struct {
	TracingEnabled    bool               `yaml:"tracing_enabled"`
	Env               string             `yaml:"env"`
	ServiceEnvConfigs []serviceEnvConfig `yaml:"service_env_configs"`
}

type tracingConfigUpdate struct {
	ID            string `json:"id"`
	Revision      int64  `json:"revision"`
	SchemaVersion string `json:"schema_version"`
	Action        string `json:"action"`
	LibConfig     struct {
		ServiceName    string `json:"service_name"`
		Env            string `json:"env"`
		TracingEnabled bool   `json:"tracing_enabled"`
	} `json:"lib_config"`
	ServiceTarget *struct {
		Service string `json:"service"`
		Env     string `json:"env"`
	} `json:"service_target"`
	InfraTarget *struct {
		Tags []string `json:"tags"`
	} `json:"infra_target"`
}

func (rc rcClient) SubscribeApmTracing() {
	if rc.client == nil {
		pkglog.Errorf("No remote-config client")
		return
	}
	rc.client.Subscribe(state.ProductAPMTracing, rc.onAPMTracingUpdate)
}

func (rc rcClient) onAPMTracingUpdate(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) { //nolint:revive
	if len(update) == 0 {
		// Empty update means revert to default behavior, so remove any existing config file
		err := os.Remove(apmTracingFilePath)
		if err == nil {
			pkglog.Infof("Removed APM_TRACING remote config file, APM injection will revert to default behavior")
		} else if !os.IsNotExist(err) {
			// If the file already wasn't there then it wasn't an error
			pkglog.Errorf("Failed to remove APM_TRACING remote config file, previous APM injection behavior will continue: %v", err)
		}
		return
	}

	var senvConfigs []serviceEnvConfig
	// Maps update IDs to their error, empty string indicates success
	updateStatus := map[string]string{}
	var hostTracingEnabled bool
	var hostEnvTarget string
	var hostConfigID string
	for id, rawConfig := range update {
		tcu := tracingConfigUpdate{}
		err := json.Unmarshal(rawConfig.Config, &tcu)
		updateStatus[id] = ""
		if err != nil {
			pkglog.Warnf("Skipping invalid APM_TRACING remote update %s: %v, any err: %v", id, tcu, err)
			updateStatus[id] = InvalidAPMTracingPayload
			continue
		}
		pkglog.Infof("Received APM_TRACING remote update %s: %v, any err: %v", id, tcu, err)
		if tcu.InfraTarget != nil {
			// This is an infra targeting payload, skip adding it to the service env config map
			if hostConfigID != "" && tcu.LibConfig.Env != hostEnvTarget {
				// We already saw a InfraTarget configuration and the envs are different, this is generally not desired
				// To be consistent we will apply the "lowest" config ID and report a failure for the un-applied host config
				pkglog.Warnf("Received more than 1 InfraTarget APM_TRACING config, the 'lowest' config will be used, but inconsistent behavior may occur. Check your Single Step Instrumentation configurations.")
				if id < hostConfigID {
					updateStatus[hostConfigID] = DuplicateHostConfig
					// fallthrough to use this update's config values
				} else {
					// The previous infra target was lower, keep the current values
					updateStatus[id] = DuplicateHostConfig
					continue
				}
			}
			hostTracingEnabled = tcu.LibConfig.TracingEnabled
			hostEnvTarget = tcu.LibConfig.Env
			hostConfigID = id
			continue
		}
		if tcu.ServiceTarget == nil {
			pkglog.Warnf("Missing service_target from APM_TRACING config update, SKIPPING: %v", tcu)
			updateStatus[id] = MissingServiceTarget
			continue
		}
		senvConfigs = append(senvConfigs, serviceEnvConfig{
			Service:        tcu.ServiceTarget.Service,
			Env:            tcu.ServiceTarget.Env,
			TracingEnabled: tcu.LibConfig.TracingEnabled,
		})
	}
	tec := tracingEnabledConfig{
		TracingEnabled:    hostTracingEnabled,
		Env:               hostEnvTarget,
		ServiceEnvConfigs: senvConfigs,
	}
	configFile, err := yamlv2.Marshal(tec)
	if err != nil {
		pkglog.Errorf("Failed to marshal APM_TRACING config update %v", err)
		return
	}
	err = os.WriteFile(apmTracingFilePath, configFile, 0644)
	if err != nil {
		pkglog.Errorf("Failed to write single step config data file from APM_TRACING config: %v", err)
		// Failed to write file, report failure for all updates
		for id := range update {
			applyStateCallback(id, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: FileWriteFailure,
			})
		}
		return
	}
	pkglog.Debugf("Successfully wrote APM_TRACING config to %s", apmTracingFilePath)
	// Successfully wrote file, report success/failure per update
	for id, errStatus := range updateStatus {
		applyState := state.ApplyStateAcknowledged
		if errStatus != "" {
			applyState = state.ApplyStateError
		}
		applyStateCallback(id, state.ApplyStatus{
			State: applyState,
			Error: errStatus,
		})
	}

}
