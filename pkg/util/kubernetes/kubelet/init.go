// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func isConfiguredCertificates() bool {
	return config.Datadog.GetString("kubelet_client_crt") != "" && config.Datadog.GetString("kubelet_client_key") != ""
}

func isConfiguredTokenPath() bool {
	return config.Datadog.GetString("kubelet_auth_token_path") != ""
}

func getCertificateAuthority() (*x509.CertPool, error) {
	certPath := config.Datadog.GetString("kubelet_client_ca")
	if certPath == "" {
		return nil, nil
	}
	return kubernetes.GetCertificateAuthority(certPath)
}

func isTLSVerify() bool {
	return config.Datadog.GetBool("kubelet_tls_verify")
}

func getTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{}
	if isTLSVerify() == false {
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}
	caPool, err := getCertificateAuthority()
	if err != nil {
		return tlsConfig, err
	}
	tlsConfig.RootCAs = caPool
	tlsConfig.BuildNameToCertificate()
	return tlsConfig, nil
}
