// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type InsecureTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *InsecureTestSuite) SetupTest() {
	kubelet.ResetGlobalKubeUtil()
}

func (suite *InsecureTestSuite) TestHTTP() {
	mockConfig := config.NewMock()

	mockConfig.Set("kubernetes_http_kubelet_port", 10255)

	// Giving 10255 http port to https setting will force an intended https discovery failure
	// Then it forces the http usage
	mockConfig.Set("kubernetes_https_kubelet_port", 10255)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), "http://127.0.0.1:10255", ku.GetKubeletApiEndpoint())
	b, code, err := ku.QueryKubelet("/healthz")
	require.Nil(suite.T(), err, fmt.Sprintf("%v", err))
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), "ok", string(b))

	b, code, err = ku.QueryKubelet("/pods")
	assert.Equal(suite.T(), 200, code)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), emptyPodList, string(b))

	podList, err := ku.GetLocalPodList()
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(podList))

	require.EqualValues(suite.T(),
		map[string]string{
			"url": "http://127.0.0.1:10255",
		}, ku.GetRawConnectionInfo())
}

func (suite *InsecureTestSuite) TestInsecureHTTPS() {
	mockConfig := config.NewMock()

	mockConfig.Set("kubernetes_http_kubelet_port", 10255)
	mockConfig.Set("kubernetes_https_kubelet_port", 10250)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", false)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "https://127.0.0.1:10250", ku.GetKubeletApiEndpoint())
	b, code, err := ku.QueryKubelet("/healthz")
	assert.Equal(suite.T(), 200, code)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "ok", string(b))

	b, code, err = ku.QueryKubelet("/pods")
	assert.Equal(suite.T(), 200, code)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), emptyPodList, string(b))

	podList, err := ku.GetLocalPodList()
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(podList))

	require.EqualValues(suite.T(),
		map[string]string{
			"url":        "https://127.0.0.1:10250",
			"verify_tls": "false",
		}, ku.GetRawConnectionInfo())
}

func TestInsecureKubeletSuite(t *testing.T) {
	compose, err := initInsecureKubelet()
	require.Nil(t, err)
	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	suite.Run(t, new(InsecureTestSuite))
}
