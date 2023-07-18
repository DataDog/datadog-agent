// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"fmt"

	logsconfig "github.com/DataDog/datadog-agent/pkg/logs/config"
)

// GetEndpointURL returns the formatted URL of the provided endpoint
func GetEndpointURL(endpoint logsconfig.Endpoint, uri string) string {
	port := endpoint.Port
	var protocol string
	if endpoint.UseSSL {
		protocol = "https"
		if port == 0 {
			port = 443 // use default port
		}
	} else {
		protocol = "http"
		if port == 0 {
			port = 80 // use default port
		}
	}
	return fmt.Sprintf("%s://%s:%v/%s", protocol, endpoint.Host, port, uri)
}
