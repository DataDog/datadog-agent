// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && orchestrator

package kubelet

import (
	"context"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

type KubeletOrchestratorTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *KubeletOrchestratorTestSuite) SetupTest() {
	mockConfig := configmock.New(suite.T())

	ResetGlobalKubeUtil()
	ResetCache()

	jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)

	mockConfig.SetWithoutSource("kubelet_client_crt", "")
	mockConfig.SetWithoutSource("kubelet_client_key", "")
	mockConfig.SetWithoutSource("kubelet_client_ca", "")
	mockConfig.SetWithoutSource("kubelet_tls_verify", true)
	mockConfig.SetWithoutSource("kubelet_auth_token_path", "")
	mockConfig.SetWithoutSource("kubernetes_kubelet_host", "")
	mockConfig.SetWithoutSource("kubernetes_http_kubelet_port", 10250)
	mockConfig.SetWithoutSource("kubernetes_https_kubelet_port", 10255)
	mockConfig.SetWithoutSource("kubernetes_pod_expiration_duration", 15*60)
}

func (suite *KubeletOrchestratorTestSuite) TestGetRawLocalPodList() {
	ctx := context.Background()
	mockConfig := configmock.New(suite.T())

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json", "")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	require.Nil(suite.T(), err)
	defer ts.Close()

	mockConfig.SetWithoutSource("kubernetes_kubelet_host", "localhost")
	mockConfig.SetWithoutSource("kubernetes_http_kubelet_port", kubeletPort)
	mockConfig.SetWithoutSource("kubelet_tls_verify", false)
	mockConfig.SetWithoutSource("kubelet_auth_token_path", "")

	kubeutil, err := GetKubeUtil()
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), kubeutil)
	kubelet.dropRequests() // Throwing away first GETs

	pods, err := kubeutil.GetRawLocalPodList(ctx)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), pods, 7)

	expectedUIDs := []string{
		"0a8863810b43d4d891fab0af80e28e4c",
		"e2fdcecc-0749-11e8-a2b8-000c29dea4f6",
		"e42b42ec-0749-11e8-a2b8-000c29dea4f6",
		"e42e5adc-0749-11e8-a2b8-000c29dea4f6",
		"7979cfcd-0751-11e8-a2b8-000c29dea4f6",
		"d91aa43c-0769-11e8-afcc-000c29dea4f6",
		"260c2b1d43b094af6d6b4ccba082c2db",
	}
	actualUIDs := make([]string, 0, len(expectedUIDs))
	for _, p := range pods {
		actualUIDs = append(actualUIDs, string(p.UID))
	}
	require.ElementsMatch(suite.T(), expectedUIDs, actualUIDs)
}

func TestKubeletOrchestratorTestSuite(t *testing.T) {
	cfg := configmock.New(t)
	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"trace",
		"",
		"",
		false,
		true,
		false,
		cfg,
	)
	suite.Run(t, new(KubeletOrchestratorTestSuite))
}
