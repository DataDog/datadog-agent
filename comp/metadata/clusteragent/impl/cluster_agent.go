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
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusteragent "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	fetchDatadogClusterAgentConfig = configFetcher.DatadogClusterAgentConfig
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname    string                 `json:"hostname"`
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

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split datadog-cluster-agent process payload any more, payload is too big for intake")
}

// Requires defines the dependencies for the clusteragent metadata component
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
}

type datadogclusteragent struct {
	util.InventoryPayload
	log          log.Component
	conf         config.Component
	hostname     string
	clustername  string
	clusterid    string
	clusteridErr string
}

// Provides defines the output of the clusteragent metadata component
type Provides struct {
	Comp             clusteragent.Component
	MetadataProvider runnerimpl.Provider
}

// NewComponent creates a new securityagent metadata Component
func NewComponent(deps Requires) Provides {
	hname, _ := hostname.Get(context.Background())
	clname := clustername.GetClusterName(context.Background(), hname)
	clid, clidErr := getClusterID()
	dca := &datadogclusteragent{
		log:          deps.Log,
		conf:         deps.Config,
		hostname:     hname,
		clustername:  clname,
		clusterid:    clid,
		clusteridErr: "",
	}
	if clidErr != nil {
		dca.clusteridErr = clidErr.Error()
	}
	dca.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, dca.getPayload, "datadog-cluster-agent.json")
	return Provides{
		Comp:             dca,
		MetadataProvider: dca.MetadataProvider(),
	}
}

func (dca *datadogclusteragent) getPayload() marshaler.JSONMarshaler {

	return &Payload{
		Hostname:    dca.hostname,
		Clustername: dca.clustername,
		ClusterID:   dca.clusterid,
		Timestamp:   time.Now().UnixNano(),
		Metadata:    dca.getMetadata(),
		UUID:        uuid.GetUUID(),
	}
}

func (dca *datadogclusteragent) initMetadata(metadata map[string]interface{}) {
	tool := "undefined"
	toolVersion := ""
	installerVersion := ""

	install, err := installinfo.Get(dca.conf)
	if err == nil {
		tool = install.Tool
		toolVersion = install.ToolVersion
		installerVersion = install.InstallerVersion
	}
	metadata["cluster_id_error"] = dca.clusteridErr
	metadata["install_method_tool"] = tool
	metadata["install_method_tool_version"] = toolVersion
	metadata["install_method_installer_version"] = installerVersion
	metadata["agent_version"] = version.AgentVersion
	metadata["agent_startup_time_ms"] = pkgconfigsetup.StartTime.UnixMilli()
	metadata["flavor"] = flavor.GetFlavor()
}

func (dca *datadogclusteragent) getAadmissionControllerConfig(metadata map[string]interface{}) {
	metadata["feature_admission_controller_enabled"] = dca.conf.GetBool("admission_controller.enabled")
	metadata["feature_admission_controller_inject_config_enabled"] = dca.conf.GetBool("admission_controller.inject_config.enabled")
	metadata["feature_admission_controller_inject_tags_enabled"] = dca.conf.GetBool("admission_controller.inject_tags.enabled")
	metadata["feature_apm_config_instrumentation_enabled"] = dca.conf.GetBool("apm_config.instrumentation.enabled")
	metadata["feature_admission_controller_validation_enabled"] = dca.conf.GetBool("admission_controller.validation.enabled")
	metadata["feature_admission_controller_mutation_enabled"] = dca.conf.GetBool("admission_controller.mutation.enabled")
	metadata["feature_admission_controller_auto_instrumentation_enabled"] = dca.conf.GetBool("admission_controller.auto_instrumentation.enabled")
	metadata["feature_admission_controller_cws_instrumentation_enabled"] = dca.conf.GetBool("admission_controller.cws_instrumentation.enabled")
	metadata["feature_cluster_checks_enabled"] = dca.conf.GetBool("cluster_checks.enabled")
	metadata["feature_autoscaling_workload_enabled"] = dca.conf.GetBool("autoscaling.workload.enabled")
	metadata["feature_external_metrics_provider_enabled"] = dca.conf.GetBool("external_metrics_provider.enabled")
	metadata["feature_external_metrics_provider_use_datadogmetric_crd"] = dca.conf.GetBool("external_metrics_provider.use_datadogmetric_crd")
	metadata["feature_compliance_config_enabled"] = dca.conf.GetBool("compliance_config.enabled")
}

func (dca *datadogclusteragent) getMetadata() map[string]interface{} {
	metadata := map[string]interface{}{}
	dca.initMetadata(metadata)
	metadata["leader_election"] = dca.conf.GetBool("leader_election")
	metadata["is_leader"] = false
	if dca.conf.GetBool("leader_election") {
		if leaderEngine, err := leaderelection.GetLeaderEngine(); err == nil {
			metadata["is_leader"] = leaderEngine.IsLeader()
		}
	}
	if str, err := fetchDatadogClusterAgentConfig(dca.conf); err == nil {
		metadata["full_configuration"] = str
	} else {
		dca.log.Debugf("error fetching datadog-cluster-agent config: %s", err)
	}
	dca.getAadmissionControllerConfig(metadata)
	return metadata
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
