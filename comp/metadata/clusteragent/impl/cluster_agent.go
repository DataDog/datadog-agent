// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package clusteragentimpl implements the clusteragent metadata providers interface
package clusteragentimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.yaml.in/yaml/v2"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusteragent "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Clustername string                 `json:"clustername"`
	ClusterID   string                 `json:"cluster_id"`
	Timestamp   int64                  `json:"timestamp"`
	Metadata    map[string]interface{} `json:"datadog_cluster_agent_metadata"`
	UUID        string                 `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// Requires defines the dependencies for the clusteragent metadata component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

type datadogclusteragent struct {
	util.InventoryPayload
	log          log.Component
	conf         config.Component
	clustername  string
	clusterid    string
	clusteridErr string
	metadata     map[string]interface{}
}

// Provides defines the output of the clusteragent metadata component
type Provides struct {
	Comp             clusteragent.Component
	MetadataProvider runnerimpl.Provider
}

// NewComponent creates a new securityagent metadata Component
func NewComponent(deps Requires) Provides {
	hname, err := deps.Hostname.Get(context.Background())
	if err != nil {
		hname = ""
	}
	clname := clustername.GetClusterName(context.Background(), hname)
	clid, clidErr := getClusterID()
	dca := &datadogclusteragent{
		log:          deps.Log,
		conf:         deps.Config,
		clustername:  clname,
		clusterid:    clid,
		clusteridErr: "",
		metadata:     make(map[string]interface{}),
	}
	if clidErr != nil {
		dca.clusteridErr = clidErr.Error()
	}
	dca.initMetadata()
	if deps.Config.GetBool("enable_cluster_agent_metadata_collection") {
		dca.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, dca.getPayload, "datadog-cluster-agent.json")
	} else {
		dca.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, nil, dca.getPayload, "datadog-cluster-agent.json")
	}
	return Provides{
		Comp:             dca,
		MetadataProvider: dca.MetadataProvider(),
	}
}

func (dca *datadogclusteragent) getPayload() marshaler.JSONMarshaler {

	return &Payload{
		Clustername: dca.clustername,
		ClusterID:   dca.clusterid,
		Timestamp:   time.Now().UnixNano(),
		Metadata:    dca.getMetadata(),
		UUID:        uuid.GetUUID(),
	}
}

func (dca *datadogclusteragent) initMetadata() {
	tool := "undefined"
	toolVersion := ""
	installerVersion := ""

	install, err := installinfo.Get(dca.conf)
	if err == nil {
		tool = install.Tool
		toolVersion = install.ToolVersion
		installerVersion = install.InstallerVersion
	}
	dca.metadata["cluster_id_error"] = dca.clusteridErr
	dca.metadata["install_method_tool"] = tool
	dca.metadata["install_method_tool_version"] = toolVersion
	dca.metadata["install_method_installer_version"] = installerVersion
	dca.metadata["agent_version"] = version.AgentVersion
	dca.metadata["agent_startup_time_ms"] = pkgconfigsetup.StartTime.UnixMilli()
	dca.metadata["flavor"] = flavor.GetFlavor()
}

func (dca *datadogclusteragent) getFeatureConfigs() {
	dca.metadata["feature_admission_controller_enabled"] = dca.conf.GetBool("admission_controller.enabled")
	dca.metadata["feature_admission_controller_inject_config_enabled"] = dca.conf.GetBool("admission_controller.inject_config.enabled")
	dca.metadata["feature_admission_controller_inject_tags_enabled"] = dca.conf.GetBool("admission_controller.inject_tags.enabled")
	dca.metadata["feature_apm_config_instrumentation_enabled"] = dca.conf.GetBool("apm_config.instrumentation.enabled")
	dca.metadata["feature_admission_controller_validation_enabled"] = dca.conf.GetBool("admission_controller.validation.enabled")
	dca.metadata["feature_admission_controller_mutation_enabled"] = dca.conf.GetBool("admission_controller.mutation.enabled")
	dca.metadata["feature_admission_controller_auto_instrumentation_enabled"] = dca.conf.GetBool("admission_controller.auto_instrumentation.enabled")
	dca.metadata["feature_admission_controller_cws_instrumentation_enabled"] = dca.conf.GetBool("admission_controller.cws_instrumentation.enabled")
	dca.metadata["feature_autoscaling_workload_enabled"] = dca.conf.GetBool("autoscaling.workload.enabled")
	dca.metadata["feature_external_metrics_provider_enabled"] = dca.conf.GetBool("external_metrics_provider.enabled")
	dca.metadata["feature_external_metrics_provider_use_datadogmetric_crd"] = dca.conf.GetBool("external_metrics_provider.use_datadogmetric_crd")
	dca.metadata["feature_compliance_config_enabled"] = dca.conf.GetBool("compliance_config.enabled")
	dca.metadata["feature_cluster_checks_enabled"] = dca.conf.GetBool("cluster_checks.enabled")
	dca.metadata["feature_cluster_checks_exclude_checks"] = dca.conf.GetStringSlice("cluster_checks.exclude_checks")
	dca.metadata["feature_cluster_checks_advanced_dispatching_enabled"] = dca.conf.GetBool("cluster_checks.advanced_dispatching_enabled")
}

func (dca *datadogclusteragent) getConfigs(data map[string]interface{}) {
	layers := dca.conf.AllSettingsBySource()
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
			if yaml, err := dca.marshalAndScrub(conf); err == nil {
				data[layer] = yaml
			}
		}
	}
	if yaml, err := dca.marshalAndScrub(dca.conf.AllSettings()); err == nil {
		data["full_configuration"] = yaml
	}
}

func (dca *datadogclusteragent) marshalAndScrub(data interface{}) (string, error) {
	flareScrubber := scrubber.NewWithDefaults()
	provided, err := yaml.Marshal(data)
	if err != nil {
		return "", dca.log.Errorf("could not marshal cluster-agent configuration: %s", err)
	}
	scrubbed, err := flareScrubber.ScrubYaml(provided)
	if err != nil {
		return "", dca.log.Errorf("could not scrubb cluster-agent configuration: %s", err)
	}
	return string(scrubbed), nil
}

func (dca *datadogclusteragent) getMetadata() map[string]interface{} {
	dca.metadata["leader_election"] = dca.conf.GetBool("leader_election")
	dca.metadata["is_leader"] = false
	if dca.conf.GetBool("leader_election") {
		if leaderEngine, err := leaderelection.GetLeaderEngine(); err == nil {
			dca.metadata["is_leader"] = leaderEngine.IsLeader()
		}
	}

	// Add cluster check runner and node agent counts
	if clcRunnerCount, nodeAgentCount, err := clusterchecks.GetNodeTypeCounts(); err == nil {
		dca.metadata["cluster_check_runner_count"] = clcRunnerCount
		dca.metadata["cluster_check_node_agent_count"] = nodeAgentCount
	}

	//Sending dca configuration can be disabled using `inventories_configuration_enabled`.
	//By default, it is true and enabled.
	if !dca.conf.GetBool("inventories_configuration_enabled") {
		return dca.metadata
	}

	dca.getConfigs(dca.metadata)
	dca.getFeatureConfigs()
	return dca.metadata
}

func getClusterID() (string, error) {
	cl, err := as.GetAPIClient()
	if err != nil {
		return "", err
	}
	coreCl := cl.Cl.CoreV1().(*corev1.CoreV1Client)
	// get clusterID
	return common.GetOrCreateClusterID(coreCl)
}

// WritePayloadAsJSON writes the payload as JSON to the response writer. It is used by cluster-agent metadata endpoint.
func (dca *datadogclusteragent) WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON calls getPayload which already scrub the data
	scrubbed, err := dca.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}
