// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet,orchestrator

package kubelet

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type KubeletOrchestratorTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *KubeletOrchestratorTestSuite) SetupTest() {
	mockConfig := config.Mock()

	ResetGlobalKubeUtil()
	ResetCache()

	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	mockConfig.Set("kubelet_client_crt", "")
	mockConfig.Set("kubelet_client_key", "")
	mockConfig.Set("kubelet_client_ca", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_wait_on_missing_container", 0)
	mockConfig.Set("kubernetes_kubelet_host", "")
	mockConfig.Set("kubernetes_http_kubelet_port", 10250)
	mockConfig.Set("kubernetes_https_kubelet_port", 10255)
	mockConfig.Set("kubernetes_pod_expiration_duration", 15*60)
}

func (suite *KubeletOrchestratorTestSuite) TestGetRawLocalPodList() {
	mockConfig := config.Mock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "localhost")
	mockConfig.Set("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	pods, err := kubeutil.GetRawLocalPodList()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), pods, 7)
}

func TestKubeletOrchestratorTestSuite(t *testing.T) {
	config.SetupLogger(
		config.LoggerName("test"),
		"trace",
		"",
		"",
		false,
		true,
		false,
	)
	suite.Run(t, new(KubeletOrchestratorTestSuite))
}
