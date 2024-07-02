// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
)

type agentConfigEndpointInfo struct {
	name     string
	scheme   string
	port     int
	endpoint string
}

func traceConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"trace-agent", "http", port, "/config"}
}

func processConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"process-agent", "http", port, "/config/all"}
}

func securityConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"security-agent", "https", port, "/agent/config"}
}

func (endpointInfo *agentConfigEndpointInfo) url() *url.URL {
	return &url.URL{
		Scheme: endpointInfo.scheme,
		Host:   net.JoinHostPort("localhost", strconv.Itoa(endpointInfo.port)),
		Path:   endpointInfo.endpoint,
	}
}

func (endpointInfo *agentConfigEndpointInfo) fetchCommand(authtoken string, osFamily osComp.Family) string {
	if osFamily == osComp.WindowsFamily {
		// This piece of code is here to mimic -SkipCertificateCheck option which is only available with PowerShell 6.0+
		return fmt.Sprintf(
			`class TrustAllCertsPolicy : System.Net.ICertificatePolicy {
				[bool] CheckValidationResult([System.Net.ServicePoint] $a,
											 [System.Security.Cryptography.X509Certificates.X509Certificate] $b,
											 [System.Net.WebRequest] $c,
											 [int] $d) {
					return $true
				}
			}
			[System.Net.ServicePointManager]::CertificatePolicy = [TrustAllCertsPolicy]::new()
			(Invoke-WebRequest -Uri "%s" -Headers @{"authorization"="Bearer %s"} -Method GET -UseBasicParsing).Content`,
			endpointInfo.url().String(),
			authtoken,
		)
	}

	// -L: follow redirects
	// -s: silent
	// -k: allow insecure server connections
	// -H: add a header
	return fmt.Sprintf(
		`curl -L -s -k -H "authorization: Bearer %s" --fail-with-body "%s"`,
		authtoken,
		endpointInfo.url().String(),
	)
}
