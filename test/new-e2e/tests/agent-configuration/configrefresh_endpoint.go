// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	osComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

type agentConfigEndpointInfo struct {
	name     string
	scheme   string
	port     int
	endpoint string
}

func traceConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"trace-agent", "https", port, "/config"}
}

func processConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"process-agent", "https", port, "/config/all"}
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

func (endpointInfo *agentConfigEndpointInfo) httpRequest(authtoken string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, endpointInfo.url().String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+authtoken)
	return req, nil
}

// hostHTTPClientWithIPCCert returns an HTTP client presenting the IPC certificate
// (mTLS), required by the trace-agent debug server.
func hostHTTPClientWithIPCCert(host *components.RemoteHost) (*http.Client, error) {
	fetchIPCCertCmd := "sudo cat /etc/datadog-agent/ipc_cert.pem"
	if host.OSFamily == osComp.WindowsFamily {
		fetchIPCCertCmd = "Get-Content -Raw -Path C:\\ProgramData\\Datadog\\ipc_cert.pem"
	}

	ipcCertContent, err := host.Execute(fetchIPCCertCmd)
	if err != nil {
		return nil, fmt.Errorf("could not read the IPC certificate file: %v", err)
	}

	// The IPC cert file holds the certificate and the key; X509KeyPair picks the
	// right PEM block from each argument.
	tlsCert, err := tls.X509KeyPair([]byte(ipcCertContent), []byte(ipcCertContent))
	if err != nil {
		return nil, fmt.Errorf("could not load the IPC certificate: %v", err)
	}

	client := host.NewHTTPClient()
	tr := client.Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.Certificates = []tls.Certificate{tlsCert}
	client.Transport = tr
	return client, nil
}
