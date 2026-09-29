// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"fmt"
	"net"
	"strconv"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostport"
)

// GetEndpointURL returns the formatted URL of the provided endpoint
func GetEndpointURL(endpoint logsconfig.Endpoint, uri string) string {
	host := endpoint.Host
	port := endpoint.Port

	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if port == 0 {
			if parsed, perr := strconv.Atoi(p); perr == nil {
				port = parsed
			}
		}
	}

	var protocol string
	if endpoint.UseSSL() {
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
	return fmt.Sprintf("%s://%s%s/%s", protocol, hostport.Join(host, strconv.Itoa(port)), endpoint.PathPrefix, uri)
}
