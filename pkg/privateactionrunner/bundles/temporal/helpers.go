// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_temporal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type temporalConnectionConfig struct {
	apiKey   string
	hostPort string
	tls      *tls.Config
}

func getTemporalConnectionConfig(
	ctx context.Context,
	credentials *privateconnection.PrivateCredentials,
) (temporalConnectionConfig, error) {
	credentialTokens := credentials.AsTokenMap()

	if address, ok := credentialTokens["address"]; ok {
		return temporalConnectionConfig{hostPort: address}, nil
	}

	connectionConfig := temporalConnectionConfig{}
	if serverAddress, ok := credentialTokens["serverAddress"]; ok {
		connectionConfig.hostPort = serverAddress
	}

	clientCertPairCrt, okCrt := credentialTokens["clientCertPairCrt"]
	clientCertPairKey, okKey := credentialTokens["clientCertPairKey"]

	if okCrt && okKey {
		cert, err := tls.X509KeyPair([]byte(clientCertPairCrt), []byte(clientCertPairKey))
		if err != nil {
			log.FromContext(ctx).Warn("Unable to parse client cert and key pair.")
			return temporalConnectionConfig{}, err
		}
		connectionConfig.tls = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}
	} else {
		connectionConfig.tls = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	if (okCrt && !okKey) || (!okCrt && okKey) {
		return temporalConnectionConfig{}, errors.New("ensure both the client certificate and client key are provided")
	}

	if serverRootCACertificate, ok := credentialTokens["serverRootCACertificate"]; ok && serverRootCACertificate != "" {
		rootCA, err := getCertPool(serverRootCACertificate)
		if err != nil {
			log.FromContext(ctx).Warn("Unable to parse root CA certificate.")
		}
		connectionConfig.tls.RootCAs = rootCA
	}

	if serverNameOverride, ok := credentialTokens["serverNameOverride"]; ok && serverNameOverride != "" {
		connectionConfig.tls.ServerName = serverNameOverride
	}

	if apiKey, ok := credentialTokens["apiKey"]; ok {
		connectionConfig.apiKey = apiKey
	}

	return connectionConfig, nil
}

func getCertPool(certString string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM([]byte(certString))
	if !ok {
		return nil, errors.New("failed to parse root certificate")
	}
	return certPool, nil
}
