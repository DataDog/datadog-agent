// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

package util

import "context"

// HostnameData contains hostname and the hostname provider
// Copy of the original struct in hostname.go
type HostnameData struct {
	Hostname string
	Provider string
}

// HostnameProviderConfiguration is the key for the hostname provider associated to datadog.yaml
// Copy of the original struct in hostname.go
const HostnameProviderConfiguration = "configuration"

// Fqdn returns the FQDN for the host if any
func Fqdn(hostname string) string {
	return ""
}

func GetHostname(ctx context.Context) (string, error) {
	return "", nil
}

func GetHostnameData() (HostnameData, error) {
	return HostnameData{}, nil
}
