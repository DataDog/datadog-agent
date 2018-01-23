// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"os"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/integration/utils"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SecureAuthQueryTestSuite struct {
	suite.Suite
	certsConfig *utils.CertificatesConfig
}

// Make sure globalKubeUtil is deleted before each test
func (suite *SecureAuthQueryTestSuite) SetupTest() {
	kubelet.ResetGlobalKubeUtil()
}

// TestSecureHTTPSKubelet with:
// - https
// - tls_verify
// - cacert
// - token
func (suite *SecureAuthQueryTestSuite) TestSecureAuthHTTPSKubelet() {
	config.Datadog.Set("kubernetes_https_kubelet_port", 10250)
	config.Datadog.Set("kubelet_auth_token_path", tokenPath)
	config.Datadog.Set("kubelet_tls_verify", true)
	config.Datadog.Set("kubelet_client_ca", certAuthPath)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.Nil(suite.T(), err, err)
	assert.Equal(suite.T(), "https://127.0.0.1:10250", ku.GetKubeletApiEndpoint())
	b, code, err := ku.QueryKubelet("/healthz")
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), "ok", string(b))

	b, code, err = ku.QueryKubelet("/pods")
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), emptyPodList, string(b))

	podList, err := ku.GetLocalPodList()
	require.Nil(suite.T(), err)
	assert.Equal(suite.T(), 0, len(podList))
}

// TestSecureHTTPSKubelet with:
// - https
// - tls_verify
// - cacert
// - WITHOUT token
func (suite *SecureAuthQueryTestSuite) TestUnauthorizedSecureHTTPSKubelet() {
	config.Datadog.Set("kubernetes_https_kubelet_port", 10250)
	config.Datadog.Set("kubelet_auth_token_path", "")
	config.Datadog.Set("kubelet_tls_verify", true)
	config.Datadog.Set("kubelet_client_ca", certAuthPath)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	_, err := kubelet.GetKubeUtil()
	require.NotNil(suite.T(), err)
	assert.True(suite.T(), strings.Contains(err.Error(), "unexpected status code 401 on endpoint https://127.0.0.1:10250/pods"))
}

func TestSecureAuthKubeletTestSuite(t *testing.T) {
	compose, certsConfig, err := initSecureKubelet(true)
	defer os.Remove(certsConfig.CertFilePath)
	defer os.Remove(certsConfig.KeyFilePath)
	require.Nil(t, err)

	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	err = createCaToken()
	defer os.Remove(tokenPath)
	defer os.Remove(certAuthPath)
	require.Nil(t, err)

	sqt := &SecureAuthQueryTestSuite{
		certsConfig: certsConfig,
	}
	suite.Run(t, sqt)
}
