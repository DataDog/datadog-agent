// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubernetes

import (
	"context"
	"fmt"
	"os"
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
	ctx := context.Background()
	mockConfig := config.Mock(nil)

	mockConfig.Set("kubernetes_https_kubelet_port", 10250)
	mockConfig.Set("kubernetes_http_kubelet_port", 10255)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_client_ca", suite.certsConfig.CertFilePath)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.NoError(suite.T(), err)
	b, code, err := ku.QueryKubelet(ctx, "/healthz")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), "ok", string(b))

	b, code, err = ku.QueryKubelet(ctx, "/pods")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), emptyPodList, string(b))

	podList, err := ku.GetLocalPodList(ctx)
	// we don't consider null podlist as valid
	require.Error(suite.T(), err)
	assert.Nil(suite.T(), podList)

	require.EqualValues(suite.T(),
		map[string]string{
			"url":        "https://127.0.0.1:10250",
			"verify_tls": "true",
			"ca_cert":    suite.certsConfig.CertFilePath,
		}, ku.GetRawConnectionInfo())
}

// TestSecureUnknownAuthHTTPSKubelet with:
// - https
// - tls_verify
// - WITHOUT cacert (expecting failure)
func (suite *SecureTestSuite) TestTLSWithoutCA() {
	mockConfig := config.Mock(nil)

	mockConfig.Set("kubernetes_https_kubelet_port", 10250)
	mockConfig.Set("kubernetes_http_kubelet_port", 10255)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_client_crt", "")
	mockConfig.Set("kubelet_client_key", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_client_ca", "")
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	_, err := kubelet.GetKubeUtil()
	require.NotNil(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "impossible to reach Kubelet with host: 127.0.0.1. Please check if your setup requires kubelet_tls_verify = false")
}

// TestTLSWithCACertificate with:
// - https
// - tls_verify
// - cacert
// - certificate
func (suite *SecureTestSuite) TestTLSWithCACertificate() {
	ctx := context.Background()
	mockConfig := config.Mock(nil)

	mockConfig.Set("kubernetes_https_kubelet_port", 10250)
	mockConfig.Set("kubernetes_http_kubelet_port", 10255)
	mockConfig.Set("kubelet_auth_token_path", "")
	mockConfig.Set("kubelet_tls_verify", true)
	mockConfig.Set("kubelet_client_crt", suite.certsConfig.CertFilePath)
	mockConfig.Set("kubelet_client_key", suite.certsConfig.KeyFilePath)
	mockConfig.Set("kubelet_client_ca", suite.certsConfig.CertFilePath)
	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")

	ku, err := kubelet.GetKubeUtil()
	require.NoError(suite.T(), err)
	b, code, err := ku.QueryKubelet(ctx, "/healthz")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), "ok", string(b))

	b, code, err = ku.QueryKubelet(ctx, "/pods")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, code)
	assert.Equal(suite.T(), emptyPodList, string(b))

	podList, err := ku.GetLocalPodList(ctx)
	// we don't consider null podlist as valid
	require.Error(suite.T(), err)
	assert.Nil(suite.T(), podList)

	require.EqualValues(suite.T(),
		map[string]string{
			"url":        "https://127.0.0.1:10250",
			"verify_tls": "true",
			"client_crt": suite.certsConfig.CertFilePath,
			"client_key": suite.certsConfig.KeyFilePath,
			"ca_cert":    suite.certsConfig.CertFilePath,
		}, ku.GetRawConnectionInfo())
}

func TestSecureKubeletSuite(t *testing.T) {
	config.SetFeatures(t, config.Kubernetes)

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
