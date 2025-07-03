// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
	"html/template"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

const (
	coreCMDPort              = 5001
	coreIPCPort              = 5004
	securityCmdPort          = 5010
	apmCmdPort               = 5012
	apmReceiverPort          = 8126
	processCmdPort           = 6162
	configRefreshIntervalSec = 10
)

//go:embed fixtures/config.yaml.tmpl
var coreConfigTmpl string

//go:embed fixtures/security-agent.yaml
var securityAgentConfig string

type endpoint struct {
	name string
	port int
}

// assertAgentUseCert checks that all agents IPC server use the IPC certificate.
func assertAgentUseCert(t *assert.CollectT, host *components.RemoteHost, ipcCertFileContent []byte) {
	// Reading and decoding cert and key from file
	var block *pem.Block

	block, rest := pem.Decode(ipcCertFileContent)
	require.NotNil(t, block)
	require.Equal(t, block.Type, "CERTIFICATE")
	cert := pem.EncodeToMemory(block)

	block, _ = pem.Decode(rest)
	require.NotNil(t, block)
	require.Equal(t, block.Type, "EC PRIVATE KEY")
	key := pem.EncodeToMemory(block)

	tlsCert, err := tls.X509KeyPair(cert, key)
	require.NoError(t, err, "Unable to generate x509 cert from PERM IPC cert and key")

	CA := x509.NewCertPool()
	ok := CA.AppendCertsFromPEM(cert)
	require.True(t, ok)

	client := host.NewHTTPClient()

	tr := client.Transport.(*http.Transport).Clone()
	// Reinitializing tlsConfig and replace transport
	tr.TLSClientConfig = &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}
	client.Transport = tr

	//Assert that it's not working if the IPC cert is not set as RootCA
	_, err = client.Get(fmt.Sprintf("https://127.0.0.1:%d", coreCMDPort)) // nolint: bodyclose
	require.Error(t, err)

	// Setting IPC certificate as Root CA
	tr.TLSClientConfig.RootCAs = CA

	for _, endpoint := range []endpoint{
		{"coreCMD", coreCMDPort},
		{"coreIPC", coreIPCPort},
		{"securityAgent", securityCmdPort},
		{"traceAgentDebug", apmCmdPort},
		{"processAgent", processCmdPort},
	} {
		// Make a request to the server
		resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d", endpoint.port))
		require.NoErrorf(t, err, "unable to connect to %v", endpoint.name)
		defer resp.Body.Close()

		require.NotNilf(t, resp.TLS, "connection to %v didn't used TLS", endpoint.name)
		require.Lenf(t, resp.TLS.PeerCertificates, 1, "server of %v server multiple certficiate", endpoint.name)
	}
}

// fillTmplConfig fills the template with the given variables and returns the result.
func fillTmplConfig(t *testing.T, tmplContent string, templateVars any) string {
	t.Helper()

	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	require.NoError(t, err)

	err = tmpl.Execute(&buffer, templateVars)
	require.NoError(t, err)

	return buffer.String()
}
