// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mysql

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	// User is the username used for authentication
	User = "root"
	// Pass is the password used for authentication
	Pass = "root"
)

// RunServer runs a MySQL server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string, withTLS bool) error {
	t.Helper()

	dir, _ := testutil.CurDir()
	cert, _, err := testutil.GetCertsPaths()
	require.NoError(t, err)
	certsDir := filepath.Dir(cert)

	env := []string{
		"MYSQL_ADDR=" + serverAddr,
		"MYSQL_PORT=" + serverPort,
		"MYSQL_ROOT_PASS=" + Pass,
		"CERTS_PATH=" + certsDir,
		"TESTDIR=" + dir,
	}

	if withTLS {
		env = append(env, "MYSQL_TLS_ARGS=--require-secure-transport --ssl-cert=/mysql-test/cert.pem.0 --ssl-key=/mysql-test/server.key")
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile(fmt.Sprintf(".*ready for connections.*port: %s.*", serverPort)), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"MYSQL",
			scanner,
			dockerutils.WithEnv(env),
		),
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}
