// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fipscompliance contains tests for the FIPS Agent runtime behavior
package fipscompliance

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cipherTestCase struct {
	cert   string
	cipher string
	tlsMin string
	tlsMax string
	want   bool
}

// fipsServer is a helper for interacting with a datadog/apps-fips-server container
type fipsServer struct {
	composeFiles string
	dockerHost   *components.RemoteHost
}

func newFIPSServer(dockerHost *components.RemoteHost, composeFiles string) fipsServer {
	s := fipsServer{
		dockerHost:   dockerHost,
		composeFiles: composeFiles,
	}
	return s
}

func (s *fipsServer) Start(t *testing.T, tc cipherTestCase) {
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// stop currently running server, if any, so we can reset logs+env
		s.Stop()

		// start datadog/apps-fips-server with env vars from the test case
		envVars := map[string]string{
			"CERT": tc.cert,
		}
		if tc.cipher != "" {
			envVars["CIPHER"] = "-c " + tc.cipher
		}
		if tc.tlsMax != "" {
			envVars["TLS_MAX"] = "--tls-max " + tc.tlsMax
		}
		if tc.tlsMin != "" {
			envVars["TLS_MIN"] = "--tls-min " + tc.tlsMin
		}

		cmd := fmt.Sprintf("docker-compose -f %s up --detach --wait --timeout 120", strings.TrimSpace(s.composeFiles))
		_, err := s.dockerHost.Execute(cmd, client.WithEnvVariables(envVars))
		if err != nil {
			t.Logf("Error starting fips-server: %v", err)
			require.NoError(c, err)
		}
		assert.Nil(c, err)
	}, 180*time.Second, 10*time.Second, "docker-compose timed out starting server")

	// Wait for container to start and ensure it's a fresh instance
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		serverLogs, _ := s.dockerHost.Execute("docker logs dd-fips-server")
		assert.Contains(c, serverLogs, "Server Starting...", "fips-server timed out waiting for cipher initialization to finish")
		assert.Equal(c, 1, strings.Count(serverLogs, "Server Starting..."), "Server should start only once, logs from previous runs should not be present")
	}, 60*time.Second, 5*time.Second)
}

func (s *fipsServer) Logs() string {
	return s.dockerHost.MustExecute("docker logs dd-fips-server")
}

func (s *fipsServer) Stop() {
	fipsContainer := s.dockerHost.MustExecute("docker container ls -a --filter name=dd-fips-server --format '{{.Names}}'")
	if fipsContainer != "" {
		s.dockerHost.MustExecute(fmt.Sprintf("docker-compose -f %s down fips-server", strings.TrimSpace(s.composeFiles)))
	}
}

var (
	testcases = []cipherTestCase{
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256", want: true},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384", want: true},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", want: true},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", want: true},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256", want: false},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA", want: false},
		{cert: "ecc", cipher: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA", want: false},
		{cert: "rsa", cipher: "TLS_AES_128_GCM_SHA256", tlsMin: "1.3", tlsMax: "1.3", want: true},
		// NOTE: TLS_AES_256_GCM_SHA384 is a supported TLS 1.3 cipher but we cannot test it with the E2E test because
		//       Go does not allow configuration of TLS 1.3 cipher suites (see: https://github.com/golang/go/issues/29349)
		//       so the fake endpoint we use for testing cannot restrict use to the AES 256 and AES 128 gets negotiated
		// {cert: "rsa", cipher: "TLS_AES_256_GCM_SHA384", tlsMin: "1.3", tlsMax: "1.3", want: true},
	}
	//go:embed fixtures/e2e_fips_test.py
	fipsTestCheck string
)

type fipsServerSuite[Env any] struct {
	e2e.BaseSuite[Env]

	fipsServer fipsServer
	// generates traffic to the FIPS server when called
	generateTestTraffic func()
}

// TestFIPSCiphers tests that generateTestTraffic communicates with fipsServer as defined
// in each test case. Some cases assert that a FIPS-compliant cipher is used, others assert that a non-FIPS cipher is not used.
func (s *fipsServerSuite[Env]) TestFIPSCiphers() {
	for _, tc := range testcases {
		s.Run(fmt.Sprintf("FIPS enabled testing '%v -c %v' (should connect %v)", tc.cert, tc.cipher, tc.want), func() {
			// Start the fips-server and waits for it to be ready
			s.fipsServer.Start(s.T(), tc)
			s.T().Cleanup(func() {
				s.fipsServer.Stop()
			})

			s.generateTestTraffic()

			serverLogs := s.fipsServer.Logs()
			if tc.want {
				assert.Contains(s.T(), serverLogs, "Negotiated cipher suite: "+tc.cipher)
			} else {
				assert.Contains(s.T(), serverLogs, "no cipher suite supported by both client and server")
			}
		})
	}
}

// TestFIPSCiphersTLSVersion tests that generateTestTraffic rejects fipsServer when the TLS version is too low
func (s *fipsServerSuite[Env]) TestFIPSCiphersTLSVersion() {
	tc := cipherTestCase{cert: "rsa", tlsMax: "1.1"}
	s.fipsServer.Start(s.T(), tc)
	s.T().Cleanup(func() {
		s.fipsServer.Stop()
	})

	s.generateTestTraffic()

	serverLogs := s.fipsServer.Logs()
	assert.Contains(s.T(), serverLogs, "tls: client offered only unsupported version")
}
