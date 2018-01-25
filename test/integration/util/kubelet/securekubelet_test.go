// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

type SecureTestSuite struct {
	suite.Suite
	certsConfig *utils.CertificatesConfig
}

// Make sure globalKubeUtil is deleted before each test
func (suite *SecureTestSuite) SetupTest() {
	kubelet.ResetGlobalKubeUtil()
}

// TestSecureHTTPSKubelet with:
// - https
// - tls_verify
// - cacert
func (suite *SecureTestSuite) TestWithTLSCA() {
	config.Datadog.Set("kubernetes_https_kubelet_port", 10250)
	config.Datadog.Set("kubernetes_http_kubelet_port", 10255)
	config.Datadog.Set("kubelet_auth_token_path", "")
	config.Datadog.Set("kubelet_tls_verify", true)
	config.Datadog.Set("kubelet_client_ca", suite.certsConfig.CertFilePath)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.Nil(suite.T(), err)
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

// TestSecureUnknownAuthHTTPSKubelet with:
// - https
// - tls_verify
// - WITHOUT cacert (expecting failure)
func (suite *SecureTestSuite) TestTLSWithoutCA() {
	config.Datadog.Set("kubernetes_https_kubelet_port", 10250)
	config.Datadog.Set("kubernetes_http_kubelet_port", 10255)
	config.Datadog.Set("kubelet_auth_token_path", "")
	config.Datadog.Set("kubelet_tls_verify", true)
	config.Datadog.Set("kubelet_client_ca", "")
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	_, err := kubelet.GetKubeUtil()
	require.NotNil(suite.T(), err)
	assert.True(suite.T(), strings.Contains(err.Error(), "Get https://127.0.0.1:10250/pods: x509: "), err.Error())
	assert.True(suite.T(), strings.Contains(err.Error(), "Get http://127.0.0.1:10255/pods: dial tcp 127.0.0.1:10255: getsockopt: connection refused"), err.Error())
}

// TestTLSWithCACertificate with:
// - https
// - tls_verify
// - cacert
// - certificate
func (suite *SecureTestSuite) TestTLSWithCACertificate() {
	config.Datadog.Set("kubernetes_https_kubelet_port", 10250)
	config.Datadog.Set("kubernetes_http_kubelet_port", 10255)
	config.Datadog.Set("kubelet_auth_token_path", "")
	config.Datadog.Set("kubelet_tls_verify", true)
	config.Datadog.Set("kubelet_client_crt", suite.certsConfig.CertFilePath)
	config.Datadog.Set("kubelet_client_ca", suite.certsConfig.CertFilePath)
	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.Nil(suite.T(), err)
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

func TestSecureKubeletSuite(t *testing.T) {
	compose, certsConfig, err := initSecureKubelet()
	defer os.Remove(certsConfig.CertFilePath)
	defer os.Remove(certsConfig.KeyFilePath)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	sqt := &SecureTestSuite{
		certsConfig: certsConfig,
	}
	suite.Run(t, sqt)
}
