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
