// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorypayload

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var installinfoGet = installinfo.Get

// Data builds the common inventory-agent fields and wire payload without a component lifecycle.
// Callers serialize access and must not mutate values after adding them to a payload.
type Data struct {
	conf         config.Component
	log          log.Component
	hostnameComp hostnameinterface.Component
	hostname     string
	data         agentMetadata
}

// NewData collects the common metadata. It does not run the full-Agent collectors.
func NewData(conf config.Component, l log.Component, hostname hostnameinterface.Component) *Data {
	hname, _ := hostname.Get(context.Background())
	d := &Data{conf: conf, log: l, hostnameComp: hostname, hostname: hname, data: make(agentMetadata)}
	d.initData()
	return d
}

func (ia *Data) initData() {
	tool := "undefined"
	toolVersion := ""
	installerVersion := ""

	install, err := installinfoGet(ia.conf)
	if err == nil {
		tool = install.Tool
		toolVersion = install.ToolVersion
		installerVersion = install.InstallerVersion
	}
	ia.data["install_method_tool"] = tool
	ia.data["install_method_tool_version"] = toolVersion
	ia.data["install_method_installer_version"] = installerVersion

	data, err := ia.hostnameComp.GetWithProvider(context.Background())
	if err == nil {
		if data.Provider != "" && !data.FromFargate() {
			ia.data["hostname_source"] = data.Provider
		}
	} else {
		ia.log.Warnf("could not fetch 'hostname_source': %v", err)
	}

	ia.data["agent_version"] = version.AgentVersion
	ia.data["package_version"] = version.AgentPackageVersion
	ia.data["agent_startup_time_ms"] = ia.conf.StartTime().UnixMilli()
	ia.data["flavor"] = flavor.GetFlavor()

	infraMode := scrub(ia.conf.GetString("infrastructure_mode"))
	// fleet-automation: This validation should be done by the Config once we have such mechanism
	if !slices.Contains([]string{"full", "end_user_device", "basic", "cloud_cost_only", "none"}, infraMode) {
		ia.log.Warnf("invalid value for 'infrastructure_mode': '%s' (defaulting to 'full')", infraMode)
		infraMode = "full"
	}
	ia.data["infrastructure_mode"] = infraMode
}

// Set stores a metadata value in the payload. The given value will be stored in the cache without being copied. It is
// up to the caller to make sure the given value will not be modified later. It returns whether the stored value changed.
func (ia *Data) Set(name string, value interface{}) bool {
	if reflect.DeepEqual(ia.data[name], value) {
		return false
	}
	ia.data[name] = value
	return true
}

// Get returns a copy of the metadata.
func (ia *Data) Get() map[string]interface{} {
	data := map[string]interface{}{}
	maps.Copy(data, ia.data)
	return data
}

func scrub(s string) string {
	// Errors come from internal use of a Reader interface. Since we are reading from a buffer, no errors
	// are possible.
	scrubString, _ := scrubber.ScrubString(s)
	return scrubString
}

func (ia *Data) marshalAndScrub(data interface{}) (string, error) {
	flareScrubber := scrubber.NewWithDefaults()

	provided, err := yaml.Marshal(data)
	if err != nil {
		return "", ia.log.Errorf("could not marshal agent configuration: %s", err)
	}

	scrubbed, err := flareScrubber.ScrubYaml(provided)
	if err != nil {
		return "", ia.log.Errorf("could not scrubb agent configuration: %s", err)
	}

	return string(scrubbed), nil
}

func (ia *Data) getConfigs(data agentMetadata) {
	if ia.conf.GetBool("inventories_configuration_enabled") {
		layers := ia.conf.AllSettingsBySource()
		layersName := map[model.Source]string{
			model.SourceFile:               "file_configuration",
			model.SourceEnvVar:             "environment_variable_configuration",
			model.SourceAgentRuntime:       "agent_runtime_configuration",
			model.SourceLocalConfigProcess: "source_local_configuration",
			model.SourceRC:                 "remote_configuration",
			model.SourceFleetPolicies:      "fleet_policies_configuration",
			model.SourceCLI:                "cli_configuration",
			model.SourceProvided:           "provided_configuration",
		}

		for source, conf := range layers {
			if layer, ok := layersName[source]; ok {
				if yaml, err := ia.marshalAndScrub(conf); err == nil {
					data[layer] = yaml
				}
			}
		}
		if yaml, err := ia.marshalAndScrub(ia.conf.AllSettingsWithoutSecrets()); err == nil {
			data["full_configuration"] = yaml
		}
	}
}

// GetPayload snapshots the common fields, overlays fields, and includes the configured scrubbed configuration.
func (ia *Data) GetPayload(fields map[string]interface{}, id string) *Payload {
	data := maps.Clone(ia.data)
	maps.Copy(data, fields)
	ia.getConfigs(data)
	return &Payload{Hostname: ia.hostname, Timestamp: time.Now().UnixNano(), Metadata: data, UUID: id}
}
