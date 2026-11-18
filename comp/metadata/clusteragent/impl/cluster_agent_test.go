// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package clusteragentimpl

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func setupClusterAgentConfig(t *testing.T) config.Component {
	conf := config.NewMock(t)
	conf.Set("admission_controller.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.inject_config.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.inject_tags.enabled", true, model.SourceUnknown)
	conf.Set("apm_config.instrumentation.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.validation.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.mutation.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.auto_instrumentation.enabled", true, model.SourceUnknown)
	conf.Set("admission_controller.cws_instrumentation.enabled", true, model.SourceUnknown)
	conf.Set("cluster_checks.enabled", true, model.SourceUnknown)
	conf.Set("autoscaling.workload.enabled", true, model.SourceUnknown)
	conf.Set("autoscaling.cluster.enabled", true, model.SourceUnknown)
	conf.Set("remote_configuration.enabled", true, model.SourceUnknown)
	conf.Set("external_metrics_provider.enabled", true, model.SourceUnknown)
	conf.Set("external_metrics_provider.use_datadogmetric_crd", true, model.SourceUnknown)
	conf.Set("compliance_config.enabled", true, model.SourceUnknown)
	return conf
}

func getClusterAgentComp(t *testing.T) *datadogclusteragent {
	l := logmock.New(t)

	cfg := setupClusterAgentConfig(t)

	r := Requires{
		Log:        l,
		Config:     cfg,
		Serializer: serializermock.NewMetricSerializer(t),
		Hostname:   hostnameimpl.NewHostnameService(),
	}

	comp := NewComponent(r).Comp
	return comp.(*datadogclusteragent)
}

func assertClusterAgentPayload(t *testing.T, metadata map[string]interface{}) {
	assert.Equal(t, true, metadata["feature_admission_controller_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_inject_config_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_inject_tags_enabled"])
	assert.Equal(t, true, metadata["feature_apm_config_instrumentation_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_validation_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_mutation_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_auto_instrumentation_enabled"])
	assert.Equal(t, true, metadata["feature_admission_controller_cws_instrumentation_enabled"])
	assert.Equal(t, true, metadata["feature_cluster_checks_enabled"])
	assert.Equal(t, true, metadata["feature_autoscaling_workload_enabled"])
	assert.Equal(t, true, metadata["feature_autoscaling_cluster_enabled"])
	assert.Equal(t, true, metadata["feature_remote_configuration_enabled"])
	assert.Equal(t, true, metadata["feature_external_metrics_provider_enabled"])
	assert.Equal(t, true, metadata["feature_external_metrics_provider_use_datadogmetric_crd"])
	assert.Equal(t, true, metadata["feature_compliance_config_enabled"])
}

func TestWritePayload(t *testing.T) {
	dca := getClusterAgentComp(t)
	req := httptest.NewRequest("GET", "http://fake_url.com", nil)
	w := httptest.NewRecorder()
	dca.WritePayloadAsJSON(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	p := Payload{}
	err = json.Unmarshal(body, &p)
	require.NoError(t, err)
	assertClusterAgentPayload(t, p.Metadata)
}

func getPayloadMetadata(t *testing.T, dca *datadogclusteragent) (map[string]interface{}, map[string]json.RawMessage) {
	body, err := dca.GetAsJSON()
	require.NoError(t, err)

	p := Payload{}
	require.NoError(t, json.Unmarshal(body, &p))

	raw := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(body, &raw))
	return p.Metadata, raw
}

func TestRCClientIDSet(t *testing.T) {
	dca := getClusterAgentComp(t)
	dca.SetRCClientID("aBc_123-xyz")

	metadata, _ := getPayloadMetadata(t, dca)
	assert.Equal(t, "aBc_123-xyz", metadata["rc_client_id"])
}

func TestRCClientIDRemoteConfigDisabled(t *testing.T) {
	// When RC is disabled the client is never created, so SetRCClientID is never called.
	dca := getClusterAgentComp(t)
	dca.conf.Set("remote_configuration.enabled", false, model.SourceUnknown)

	metadata, _ := getPayloadMetadata(t, dca)
	id, ok := metadata["rc_client_id"]
	require.True(t, ok, "rc_client_id should always be present")
	assert.Equal(t, "", id)
}

func TestRCClientIDPayloadShape(t *testing.T) {
	dca := getClusterAgentComp(t)
	dca.SetRCClientID("client-id")

	_, raw := getPayloadMetadata(t, dca)
	// rc_client_id lives in datadog_cluster_agent_metadata, next to pod_name and is_leader,
	// not at the top level of the payload.
	_, topLevel := raw["rc_client_id"]
	assert.False(t, topLevel)

	var metadata map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["datadog_cluster_agent_metadata"], &metadata))
	assert.JSONEq(t, `"client-id"`, string(metadata["rc_client_id"]))
	assert.Contains(t, metadata, "pod_name")
	assert.Contains(t, metadata, "is_leader")
}
