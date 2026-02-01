// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package cert

import (
	"crypto/x509"
)

// Note: This function is a no-op when kubeapiserver is not compiled.
// The non core-agents will not be able to communicate with the Cluster Agent.
// Cannot throw an error because will cause sub-agents to fail to start.
func fetchClusterCAFromSecret(_, _ string) (*x509.Certificate, error) {
	return nil, nil
}
