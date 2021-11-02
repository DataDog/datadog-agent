// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package testutil

import "fmt"

// OTLPConfigFromPorts creates a test OTLP config map.
func OTLPConfigFromPorts(bindHost string, gRPCPort uint, httpPort uint) map[string]interface{} {
	otlpConfig := map[string]interface{}{"protocols": map[string]interface{}{}}

	if gRPCPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["grpc"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, gRPCPort),
		}
	}
	if httpPort > 0 {
		otlpConfig["protocols"].(map[string]interface{})["http"] = map[string]interface{}{
			"endpoint": fmt.Sprintf("%s:%d", bindHost, httpPort),
		}
	}
	return otlpConfig
}
