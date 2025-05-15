// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package flare

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingWorkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	autoscalingWorkloadModel "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetAutoscalerList(t *testing.T) {
	dpaInternal := autoscalingWorkloadModel.NewFakePodAutoscalerInternal("ns", "test-dpa", nil)
	autoscalerInfo := autoscalingWorkload.AutoscalersInfo{
		PodAutoscalers: []autoscalingWorkloadModel.PodAutoscalerInternal{
			dpaInternal,
		},
	}

	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		out, _ := json.Marshal(&autoscalerInfo)
		w.Write(out)
	}))
	defer s.Close()

	setupIPCAddress(t, configmock.New(t), s.URL)

	content, err := GetAutoscalerList(s.URL)
	require.NoError(t, err)

	assert.Contains(t, string(content), "test-dpa")
}
