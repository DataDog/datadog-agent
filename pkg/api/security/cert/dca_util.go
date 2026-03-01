// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cert

import (
	"crypto/x509"
	"encoding/pem"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// fetchClusterCAFromSecret fetches the CA certificate from a Kubernetes Secret.
// This is used for Fargate sidecars that cannot mount secrets from other namespaces.
// It only fetches the public CA certificate (tls.crt), not the private key.
func fetchClusterCAFromSecret(secretNamespace, secretName string) (*x509.Certificate, error) {
	secretData, err := apiserver.GetKubeSecret(secretNamespace, secretName)
	if err != nil {
		return nil, errors.New("failed to fetch CA secret " + secretNamespace + "/" + secretName + ": " + err.Error())
	}

	caCertPEM, ok := secretData["tls.crt"]
	if !ok {
		return nil, errors.New("secret " + secretNamespace + "/" + secretName + " does not contain 'tls.crt' key")
	}

	block, _ := pem.Decode(caCertPEM)
	if block == nil {
		return nil, errors.New("failed to decode CA certificate PEM from secret")
	}

	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("failed to parse CA certificate: " + err.Error())
	}

	log.Infof("Successfully loaded cluster CA from secret %s/%s", secretNamespace, secretName)
	return caCert, nil
}
