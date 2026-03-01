// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package hostname

const (
	// ConfigProvider is the provider name used when the hostname is found in the configuration file.
	ConfigProvider = "configuration"

	// FargateProvider is the provider name used when running in Fargate/sidecar mode.
	FargateProvider = "fargate"
)

// FromConfiguration returns true if the hostname was found through the configuration file.
func (h Data) FromConfiguration() bool {
	return h.Provider == ConfigProvider
}

// FromFargate returns true if the hostname was found through Fargate.
func (h Data) FromFargate() bool {
	return h.Provider == FargateProvider
}
