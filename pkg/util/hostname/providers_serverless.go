// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

package hostname

import "context"

// HostnameProviderConfiguration is the key for the hostname provider associated to datadog.yaml
// Copy of the original struct in hostname.go
const HostnameProviderConfiguration = "configuration"

func GetWithProvider(ctx context.Context) (string, string, error) {
	return "", "", nil
}

func Get() (string, error) {
	return "", nil
}
