// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package redis provides a Redis client to interact with a Redis server.
package redis

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunServer runs a Redis server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string, enableTLS bool) error {
	t.Helper()
	dir, _ := testutil.CurDir()

	cert, _, err := testutil.GetCertsPaths()
	require.NoError(t, err)
	certsDir := filepath.Dir(cert)

	env := []string{
		"REDIS_ADDR=" + serverAddr,
		"REDIS_PORT=" + serverPort,
		"CERTS_PATH=" + certsDir,
		"TESTDIR=" + dir,
	}

	if enableTLS {
		args := fmt.Sprintf("REDIS_ARGS=--tls-port %v --port 0 --tls-cert-file /redis-test/cert.pem.0 --tls-key-file /redis-test/server.key --tls-auth-clients no", serverPort)
		env = append(env, args)
	}

	return protocolsUtils.RunDockerServer(t, "redis", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(".*Ready to accept connections"), protocolsUtils.DefaultTimeout, 3)
}
