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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	CoreCMDPort              = 5001
	CoreIPCPort              = 5004
	SecurityCmdPort          = 5010
	ApmCmdPort               = 5012
	ApmReceiverPort          = 8126
	ProcessCmdPort           = 6162
	ConfigRefreshIntervalSec = 10
)

//go:embed fixtures/config.yaml.tmpl
var CoreConfigTmpl string

//go:embed fixtures/security-agent.yaml
var SecurityAgentConfig string

type Endpoint struct {
	Name string
	Port int
}

// AssertAgentUseCert checks that all agents IPC server use the IPC certificate.
func AssertAgentUseCert(t *assert.CollectT, host *components.RemoteHost, ipcCertFileContent []byte) {
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
	_, err = client.Get(fmt.Sprintf("https://127.0.0.1:%d", CoreCMDPort)) // nolint: bodyclose
	require.Error(t, err)

	// Setting IPC certificate as Root CA
	tr.TLSClientConfig.RootCAs = CA

	for _, endpoint := range []Endpoint{
		{"coreCMD", CoreCMDPort},
		{"coreIPC", CoreIPCPort},
		{"securityAgent", SecurityCmdPort},
		{"traceAgentDebug", ApmCmdPort},
		{"processAgent", ProcessCmdPort},
	} {
		// Make a request to the server
		resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d", endpoint.Port))
		require.NoErrorf(t, err, "unable to connect to %v", endpoint.Name)
		defer resp.Body.Close()

		require.NotNilf(t, resp.TLS, "connection to %v didn't used TLS", endpoint.Name)
		require.Lenf(t, resp.TLS.PeerCertificates, 1, "server of %v server multiple certficiate", endpoint.Name)
	}
}

// FillTmplConfig fills the template with the given variables and returns the result.
func FillTmplConfig(t *testing.T, tmplContent string, templateVars any) string {
	t.Helper()

	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	require.NoError(t, err)

	err = tmpl.Execute(&buffer, templateVars)
	require.NoError(t, err)

	return buffer.String()
}
