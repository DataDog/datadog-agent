// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryagentimpl implements a component to generate the 'datadog_agent' metadata payload for inventory.
package inventoryagentimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	iainterface "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newInventoryAgentProvider))
}

var (
	// for testing
	installinfoGet = installinfo.Get
)

type agentMetadata map[string]interface{}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string        `json:"hostname"`
	Timestamp int64         `json:"timestamp"`
	Metadata  agentMetadata `json:"agent_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
}

type inventoryagent struct {
	util.InventoryPayload

	log          log.Component
	conf         config.Component
	sysprobeConf optional.Option[sysprobeconfig.Component]
	m            sync.Mutex
	data         agentMetadata
	hostname     string
}

type dependencies struct {
	fx.In

	Log            log.Component
	Config         config.Component
	SysProbeConfig optional.Option[sysprobeconfig.Component]
	Serializer     serializer.MetricSerializer
}

type provides struct {
	fx.Out

	Comp                 iainterface.Component
	Provider             runnerimpl.Provider
	FlareProvider        flaretypes.Provider
	StatusHeaderProvider status.HeaderInformationProvider
}

func newInventoryAgentProvider(deps dependencies) provides {
	hname, _ := hostname.Get(context.Background())
	ia := &inventoryagent{
		conf:         deps.Config,
		sysprobeConf: deps.SysProbeConfig,
		log:          deps.Log,
		hostname:     hname,
		data:         make(agentMetadata),
	}
	ia.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, ia.getPayload, "agent.json")

	if ia.Enabled {
		ia.initData()
		// We want to be notified when the configuration is updated
		deps.Config.OnUpdate(func(_ string) { ia.Refresh() })
	}

	return provides{
		Comp:                 ia,
		Provider:             ia.MetadataProvider(),
		FlareProvider:        ia.FlareProvider(),
		StatusHeaderProvider: status.NewHeaderInformationProvider(ia),
	}
}

func (ia *inventoryagent) initData() {
	clean := func(s string) string {
		// Errors come from internal use of a Reader interface. Since we are reading from a buffer, no errors
		// are possible.
		cleanBytes, _ := scrubber.ScrubBytes([]byte(s))
		return string(cleanBytes)
	}

	cfgSlice := func(name string) []string {
		if ia.conf.IsSet(name) {
			ss := ia.conf.GetStringSlice(name)
			rv := make([]string, len(ss))
			for i, s := range ss {
				rv[i] = clean(s)
			}
			return rv
		}
		return []string{}
	}

	// if system probe configuration was loaded we use it, if not we default to false
	getBoolSysProbe := func(key string) bool { return false }
	getIntSysProbe := func(key string) int { return 0 }
	if sysprobeConf, isset := ia.sysprobeConf.Get(); isset {
		getBoolSysProbe = func(key string) bool { return sysprobeConf.GetBool(key) }
		getIntSysProbe = func(key string) int { return sysprobeConf.GetInt(key) }
	}

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

	data, err := hostname.GetWithProvider(context.Background())
	if err == nil {
		if data.Provider != "" && !data.FromFargate() {
			ia.data["hostname_source"] = data.Provider
		}
	} else {
		ia.log.Warnf("could not fetch 'hostname_source': %v", err)
	}

	ia.data["agent_version"] = version.AgentVersion
	ia.data["flavor"] = flavor.GetFlavor()

	ia.data["config_apm_dd_url"] = clean(ia.conf.GetString("apm_config.apm_dd_url"))
	ia.data["config_dd_url"] = clean(ia.conf.GetString("dd_url"))
	ia.data["config_site"] = clean(ia.conf.GetString("dd_site"))
	ia.data["config_logs_dd_url"] = clean(ia.conf.GetString("logs_config.logs_dd_url"))
	ia.data["config_logs_socks5_proxy_address"] = clean(ia.conf.GetString("logs_config.socks5_proxy_address"))
	ia.data["config_no_proxy"] = cfgSlice("proxy.no_proxy")
	ia.data["config_process_dd_url"] = clean(ia.conf.GetString("process_config.process_dd_url"))
	ia.data["config_proxy_http"] = clean(ia.conf.GetString("proxy.http"))
	ia.data["config_proxy_https"] = clean(ia.conf.GetString("proxy.https"))

	ia.data["feature_fips_enabled"] = ia.conf.GetBool("fips.enabled")
	ia.data["feature_logs_enabled"] = ia.conf.GetBool("logs_enabled")
	ia.data["feature_cspm_enabled"] = ia.conf.GetBool("compliance_config.enabled")
	ia.data["feature_cspm_host_benchmarks_enabled"] = ia.conf.GetBool("compliance_config.host_benchmarks.enabled")
	ia.data["feature_apm_enabled"] = ia.conf.GetBool("apm_config.enabled")
	ia.data["feature_imdsv2_enabled"] = ia.conf.GetBool("ec2_prefer_imdsv2")
	ia.data["feature_dynamic_instrumentation_enabled"] = getBoolSysProbe("dynamic_instrumentation.enabled")
	ia.data["feature_remote_configuration_enabled"] = ia.conf.GetBool("remote_configuration.enabled")

	ia.data["feature_container_images_enabled"] = ia.conf.GetBool("container_image.enabled")

	ia.data["feature_cws_enabled"] = getBoolSysProbe("runtime_security_config.enabled")
	ia.data["feature_cws_network_enabled"] = getBoolSysProbe("event_monitoring_config.network.enabled")
	ia.data["feature_cws_security_profiles_enabled"] = getBoolSysProbe("runtime_security_config.activity_dump.enabled")
	ia.data["feature_cws_remote_config_enabled"] = getBoolSysProbe("runtime_security_config.remote_configuration.enabled")

	ia.data["feature_csm_vm_containers_enabled"] = ia.conf.GetBool("sbom.enabled") && ia.conf.GetBool("container_image.enabled") && ia.conf.GetBool("sbom.container_image.enabled")
	ia.data["feature_csm_vm_hosts_enabled"] = ia.conf.GetBool("sbom.enabled") && ia.conf.GetBool("sbom.host.enabled")

	ia.data["feature_process_enabled"] = ia.conf.GetBool("process_config.process_collection.enabled")
	ia.data["feature_process_language_detection_enabled"] = ia.conf.GetBool("language_detection.enabled")
	ia.data["feature_processes_container_enabled"] = ia.conf.GetBool("process_config.container_collection.enabled")

	ia.data["feature_networks_enabled"] = getBoolSysProbe("network_config.enabled")
	ia.data["feature_networks_http_enabled"] = getBoolSysProbe("service_monitoring_config.enable_http_monitoring")
	ia.data["feature_networks_https_enabled"] = getBoolSysProbe("service_monitoring_config.tls.native.enabled")

	ia.data["feature_usm_enabled"] = getBoolSysProbe("service_monitoring_config.enabled")
	ia.data["feature_usm_kafka_enabled"] = getBoolSysProbe("service_monitoring_config.enable_kafka_monitoring")
	ia.data["feature_usm_java_tls_enabled"] = getBoolSysProbe("service_monitoring_config.tls.java.enabled")
	ia.data["feature_usm_http2_enabled"] = getBoolSysProbe("service_monitoring_config.enable_http2_monitoring")
	ia.data["feature_usm_istio_enabled"] = getBoolSysProbe("service_monitoring_config.tls.istio.enabled")
	ia.data["feature_usm_http_by_status_code_enabled"] = getBoolSysProbe("service_monitoring_config.enable_http_stats_by_status_code")
	ia.data["feature_usm_go_tls_enabled"] = getBoolSysProbe("service_monitoring_config.tls.go.enabled")

	ia.data["feature_tcp_queue_length_enabled"] = getBoolSysProbe("system_probe_config.enable_tcp_queue_length")
	ia.data["feature_oom_kill_enabled"] = getBoolSysProbe("system_probe_config.enable_oom_kill")
	ia.data["feature_windows_crash_detection_enabled"] = getBoolSysProbe("windows_crash_detection.enabled")

	ia.data["system_probe_core_enabled"] = getBoolSysProbe("system_probe_config.enable_co_re")
	ia.data["system_probe_runtime_compilation_enabled"] = getBoolSysProbe("system_probe_config.enable_runtime_compiler")
	ia.data["system_probe_kernel_headers_download_enabled"] = getBoolSysProbe("system_probe_config.enable_kernel_header_download")
	ia.data["system_probe_prebuilt_fallback_enabled"] = getBoolSysProbe("system_probe_config.allow_precompiled_fallback")
	ia.data["system_probe_telemetry_enabled"] = getBoolSysProbe("system_probe_config.telemetry_enabled")
	ia.data["system_probe_max_connections_per_message"] = getIntSysProbe("system_probe_config.max_conns_per_message")
	ia.data["system_probe_track_tcp_4_connections"] = getBoolSysProbe("network_config.collect_tcp_v4")
	ia.data["system_probe_track_tcp_6_connections"] = getBoolSysProbe("network_config.collect_tcp_v6")
	ia.data["system_probe_track_udp_4_connections"] = getBoolSysProbe("network_config.collect_udp_v4")
	ia.data["system_probe_track_udp_6_connections"] = getBoolSysProbe("network_config.collect_udp_v6")
	ia.data["system_probe_protocol_classification_enabled"] = getBoolSysProbe("network_config.enable_protocol_classification")
	ia.data["system_probe_gateway_lookup_enabled"] = getBoolSysProbe("network_config.enable_gateway_lookup")
	ia.data["system_probe_root_namespace_enabled"] = getBoolSysProbe("network_config.enable_root_netns")
}

// Set updates a metadata value in the payload. The given value will be stored in the cache without being copied. It is
// up to the caller to make sure the given value will not be modified later.
func (ia *inventoryagent) Set(name string, value interface{}) {
	if !ia.Enabled {
		return
	}

	ia.log.Debugf("setting inventory agent metadata '%s': '%v'", name, value)

	ia.m.Lock()
	defer ia.m.Unlock()

	if !reflect.DeepEqual(ia.data[name], value) {
		ia.data[name] = value
		ia.Refresh()
	}
}

func (ia *inventoryagent) getPayload() marshaler.JSONMarshaler {
	ia.m.Lock()
	defer ia.m.Unlock()

	// Create a static copy of agentMetadata for the payload
	data := make(agentMetadata)
	for k, v := range ia.data {
		data[k] = v
	}

	configLayer := map[string]func() (string, error){
		"full_configuration":                 ia.getFullConfiguration,
		"provided_configuration":             ia.getProvidedConfiguration,
		"file_configuration":                 ia.getFileConfiguration,
		"environment_variable_configuration": ia.getEnvVarConfiguration,
		"agent_runtime_configuration":        ia.getRuntimeConfiguration,
		"remote_configuration":               ia.getRemoteConfiguration,
		"cli_configuration":                  ia.getCliConfiguration,
	}
	for layer, getter := range configLayer {
		if conf, err := getter(); err == nil {
			data[layer] = conf
		}
	}

	return &Payload{
		Hostname:  ia.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  data,
	}
}

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (ia *inventoryagent) Get() map[string]interface{} {
	ia.m.Lock()
	defer ia.m.Unlock()

	data := map[string]interface{}{}
	for k, v := range ia.data {
		data[k] = v
	}
	return data
}
