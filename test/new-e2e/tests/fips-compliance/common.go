// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fipscompliance contains tests for the FIPS Agent runtime behavior
package fipscompliance

import (
	_ "embed"
	"errors"
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

const (
	// fipsComposeWaitTimeoutSec is compose --wait-timeout: how long the container may take to become ready.
	// Compose --timeout is only the shutdown grace period, it does not bound startup.
	fipsComposeWaitTimeoutSec = 60
	// fipsComposeShutdownTimeoutSec is compose --timeout, applied when up replaces a running container.
	fipsComposeShutdownTimeoutSec = 30
	// fipsComposeUpTimeoutSec kills a compose up that never returns, for example a stalled image pull.
	// It leaves headroom over the 60 s readiness wait for pulling and creating the container, so a slow
	// readiness check fails inside compose with compose's own error.
	// The deadline has to be enforced on the host: ssh Execute takes no context and a require.EventuallyWithT
	// timeout abandons a running call instead of cancelling it.
	fipsComposeUpTimeoutSec = 90
	// fipsComposeDownTimeoutSec bounds compose down for the same reason.
	fipsComposeDownTimeoutSec = 60
	// fipsStartAttempts is the number of times Start tries compose up before failing.
	fipsStartAttempts = 2

	fipsAppsImageRepo = "/apps-fips-server"
	// fipsServerComposeService is the service name in the shared fixtures. The cluster agent fixture names the
	// service dd-fips-server instead. The container name is dd-fips-server in every fixture.
	fipsServerComposeService = "fips-server"
)

// fipsServer is a helper for interacting with a datadog/apps-fips-server container.
type fipsServer struct {
	composeFiles string
	dockerHost   *components.RemoteHost
	// service is the compose service to start and stop. Defaults to fipsServerComposeService.
	service string
	// extraEnv is added to every compose call. The cluster agent fixture interpolates CLUSTER_AGENT_IMAGE with no default.
	extraEnv map[string]string
	// appsRegistry is the registry the provisioned fips server image came from. Compose calls made by the test
	// do not inherit the environment Pulumi used, so without it the fixtures fall back to ghcr.io.
	appsRegistry string
}

func newFIPSServer(t testing.TB, dockerHost *components.RemoteHost, composeFiles string) fipsServer {
	s := fipsServer{
		dockerHost:   dockerHost,
		composeFiles: composeFiles,
	}
	s.appsRegistry = s.provisionedAppsRegistry()
	if s.appsRegistry == "" {
		t.Logf("no provisioned apps registry found, docker-compose uses its default for apps-fips-server")
	}
	return s
}

// appsRegistryFromImage returns the registry prefix of an apps-fips-server image reference, or an empty string
// when the reference is not that image.
func appsRegistryFromImage(image string) string {
	image = strings.Trim(strings.TrimSpace(image), `'"`)
	i := strings.LastIndex(image, fipsAppsImageRepo)
	if i <= 0 {
		return ""
	}
	if rest := image[i+len(fipsAppsImageRepo):]; rest != "" && rest[0] != ':' && rest[0] != '@' {
		return ""
	}
	return image[:i]
}

// provisionedAppsRegistry reads the registry from the fips server container created at provisioning time.
// It returns an empty string when no such container exists, in which case compose keeps its default registry.
func (s *fipsServer) provisionedAppsRegistry() string {
	image, err := s.dockerHost.Execute("docker inspect --format '{{.Config.Image}}' dd-fips-server")
	if err != nil {
		return ""
	}
	return appsRegistryFromImage(image)
}

func (s *fipsServer) composeEnv(extra map[string]string) map[string]string {
	env := make(map[string]string, len(s.extraEnv)+len(extra)+1)
	for k, v := range s.extraEnv {
		env[k] = v
	}
	if s.appsRegistry != "" {
		env["DD_APPS_REGISTRY"] = s.appsRegistry
	}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func (s *fipsServer) composeService() string {
	if s.service != "" {
		return s.service
	}
	return fipsServerComposeService
}

// startOnce stops any running server, then starts a fresh one with the test case's cert, cipher and TLS limits.
func (s *fipsServer) startOnce(t testing.TB, tc cipherTestCase) error {
	// stop currently running server, if any, so we can reset logs+env
	if err := s.stop(t); err != nil {
		return err
	}

	envVars := s.composeEnv(map[string]string{"CERT": tc.cert})
	if tc.cipher != "" {
		envVars["CIPHER"] = "-c " + tc.cipher
	}
	if tc.tlsMax != "" {
		envVars["TLS_MAX"] = "--tls-max " + tc.tlsMax
	}
	if tc.tlsMin != "" {
		envVars["TLS_MIN"] = "--tls-min " + tc.tlsMin
	}

	cmd := fmt.Sprintf(
		"timeout -k 15 %d docker-compose -f %s up --detach --wait --wait-timeout %d --timeout %d %s",
		fipsComposeUpTimeoutSec,
		strings.TrimSpace(s.composeFiles),
		fipsComposeWaitTimeoutSec,
		fipsComposeShutdownTimeoutSec,
		s.composeService(),
	)
	_, err := s.dockerHost.Execute(cmd, client.WithEnvVariables(envVars))
	return err
}

func (s *fipsServer) Start(t *testing.T, tc cipherTestCase) {
	// A bounded loop instead of require.EventuallyWithT: every attempt is bounded on the host by timeout(1),
	// so no compose call can still be running when the next attempt, or the next subtest, starts.
	// Each attempt begins with stop(), so a retry needs no delay to start from a clean state.
	var err error
	for attempt := 1; attempt <= fipsStartAttempts; attempt++ {
		if err = s.startOnce(t, tc); err == nil {
			break
		}
		t.Logf("Error starting fips-server (attempt %d of %d): %v", attempt, fipsStartAttempts, err)
	}
	require.NoError(t, err, "docker-compose failed to start the fips server")

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

// stop removes the fips server container if there is one. Compose down is bounded on the host. When it fails, the
// error is logged and the container is removed with plain docker. A container that is already gone counts as removed,
// because a killed compose down may have removed it before failing.
func (s *fipsServer) stop(t testing.TB) error {
	name, err := s.dockerHost.Execute("docker container ls -a --filter name=dd-fips-server --format '{{.Names}}'")
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return nil
	}
	cmd := fmt.Sprintf(
		"timeout -k 15 %d docker-compose -f %s down %s",
		fipsComposeDownTimeoutSec,
		strings.TrimSpace(s.composeFiles),
		s.composeService(),
	)
	_, downErr := s.dockerHost.Execute(cmd, client.WithEnvVariables(s.composeEnv(nil)))
	if downErr == nil {
		return nil
	}
	t.Logf("docker-compose down failed, removing the container with docker: %v", downErr)
	if _, rmErr := s.dockerHost.Execute("docker rm -f dd-fips-server || ! docker container inspect dd-fips-server >/dev/null 2>&1"); rmErr != nil {
		return errors.Join(fmt.Errorf("docker-compose down: %w", downErr), fmt.Errorf("docker rm: %w", rmErr))
	}
	return nil
}

// Stop removes the fips server container. It is meant for test cleanup, so a failure is reported without aborting.
func (s *fipsServer) Stop(t *testing.T) {
	assert.NoError(t, s.stop(t), "failed to remove the fips server container")
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
			// capture the subtest's T: suite.Run restores the parent T before subtest cleanups run
			t := s.T()
			s.fipsServer.Start(t, tc)
			t.Cleanup(func() {
				s.fipsServer.Stop(t)
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
	t := s.T()
	s.fipsServer.Start(t, tc)
	t.Cleanup(func() {
		s.fipsServer.Stop(t)
	})

	s.generateTestTraffic()

	serverLogs := s.fipsServer.Logs()
	assert.Contains(s.T(), serverLogs, "tls: client offered only unsupported version")
}
