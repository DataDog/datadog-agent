// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.temporal.io/sdk/client"

	credssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/credentials"
)

func newTemporalClient(ctx context.Context, credentials interface{}, namespace string) (client.Client, error) {
	connectionOptions, err := getTemporalConnectionOptions(ctx, credentials, namespace)
	if err != nil {
		return nil, err
	}
	temporalClient, err := client.NewClient(connectionOptions)
	if err != nil {
		return nil, err
	}
	return temporalClient, nil
}

func getTemporalConnectionOptions(ctx context.Context, credentials interface{}, namespace string) (client.Options, error) {
	clientOptions := client.Options{
		Namespace: namespace,
	}
	credentialTokens, err := credssupport.ToTokensMap(credentials)
	if err != nil {
		return client.Options{}, err
	}

	// Simple address connection case
	if address, ok := credentialTokens["address"]; ok {
		clientOptions.HostPort = address
		return clientOptions, nil
	}
	// TLS and mTLS connection case
	if serverAddress, ok := credentialTokens["serverAddress"]; ok {
		clientOptions.HostPort = serverAddress
	}
	clientCertPairCrt, okCrt := credentialTokens["clientCertPairCrt"]
	clientCertPairKey, okKey := credentialTokens["clientCertPairKey"]

	// mTLS connection case
	if okCrt && okKey {
		cert, err := tls.X509KeyPair([]byte(clientCertPairCrt), []byte(clientCertPairKey))
		if err != nil {
			log.Warn("Unable to parse client cert and key pair.")
			return client.Options{}, err
		}
		clientOptions.ConnectionOptions = client.ConnectionOptions{
			TLS: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cert},
			},
		}
	} else {
		clientOptions.ConnectionOptions = client.ConnectionOptions{
			TLS: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
	}

	if (okCrt && !okKey) || (!okCrt && okKey) {
		return client.Options{}, fmt.Errorf("ensure both the client certificate and client key are provided")
	}

	// additional optional configuration for TLS and mTLS
	if serverRootCACertificate, ok := credentialTokens["serverRootCACertificate"]; ok {
		rootCA, err := getCertPool(serverRootCACertificate)
		if err != nil {
			log.Warn("Unable to parse root CA certificate.")
		}
		clientOptions.ConnectionOptions.TLS.RootCAs = rootCA
	}

	if serverNameOverride, ok := credentialTokens["serverNameOverride"]; ok {
		clientOptions.ConnectionOptions.TLS.ServerName = serverNameOverride
	}

	return clientOptions, nil
}

func getCertPool(certString string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM([]byte(certString))
	if !ok {
		return nil, fmt.Errorf("failed to parse root certificate")
	}
	return certPool, nil
}
