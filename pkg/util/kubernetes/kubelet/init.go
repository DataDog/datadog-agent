// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/tls"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func isCertificatesConfigured() bool {
	return config.Datadog.GetString("kubelet_client_crt") != "" && config.Datadog.GetString("kubelet_client_key") != ""
}

func isTokenPathConfigured() bool {
	return config.Datadog.GetString("kubelet_auth_token_path") != ""
}

func isConfiguredTLSVerify() bool {
	return config.Datadog.GetBool("kubelet_tls_verify")
}

func buildTLSConfig(verifyTLS bool, caPath string) (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if verifyTLS == false {
		log.Info("Skipping TLS verification")
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}

	if caPath == "" {
		log.Debug("kubelet_client_ca isn't configured: certificate authority must be trusted")
		return nil, nil
	}

	caPool, err := kubernetes.GetCertificateAuthority(caPath)
	if err != nil {
		return tlsConfig, err
	}
	tlsConfig.RootCAs = caPool
	tlsConfig.BuildNameToCertificate()
	return tlsConfig, nil
}
