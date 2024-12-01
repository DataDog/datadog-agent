// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// coreimpl provide functions ...
package coreimpl

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"path/filepath"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ipc/security/cert"
)

// func NewIPCServer(s *http.Server, options ...server.Option) (server.Server, error) {
// 	return server.NewIPCServer(s, NewConfigResolver(), options...)
// }

func getCertPath() string {
	config := pkgconfigsetup.Datadog()
	if configPath := config.GetString("ipc_cert_file_path"); configPath != "" {
		return configPath
	}
	return filepath.Join(filepath.Dir(config.ConfigFileUsed()), "ipc_cert")
}

// GetServerTLSConfig provide a tls.Config that dynamically resolve
// the IPC certificate use to prove identity to API clients.
// Under the hood it retrieve or create the certificate file
func GetServerTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, key, err := cert.CreateOrFetchAgentIPCCert(getCertPath())
			if err != nil {
				return nil, err
			}

			// Create a TLS certificate using the generated cert and key
			tlsCert, err := tls.X509KeyPair(cert, key)
			if err != nil {
				return nil, err
			}
			return &tlsCert, nil
		},
	}
}

// GetClientTLSConfig provide a tls.Config that dynamically resolve
// the IPC certificate use to prove identity to API clients.
// Under the hood it retrieve or create the certificate file
func GetClientTLSConfig() (*tls.Config, error) {

	// Using Fetch function and not CreateOrFetch because if the cert haven't been created yet
	// it means that no IPC servers has started yet.
	cert, _, err := cert.CreateOrFetchAgentIPCCert(getCertPath())
	if err != nil {
		return nil, err
	}

	// Create a certificate pool and add the self-signed certificate
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(cert); !ok {
		return nil, fmt.Errorf("unable to read cert //TODO")
	}

	return &tls.Config{
		RootCAs: certPool,
	}, nil
}
