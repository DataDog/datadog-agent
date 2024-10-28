// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package postgres provides a simple wrapper around 3rd party postgres client.
package postgres

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunServer runs a postgres server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string, enableTLS bool) error {
	t.Helper()

	encryptionMode := "off"
	if enableTLS {
		encryptionMode = "on"
	}

	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	dir, _ := testutil.CurDir()
	testDataDir := filepath.Join(dir, "testdata")
	if err := linkFile(t, key, filepath.Join(testDataDir, "server.key")); err != nil {
		return err
	}
	if err := linkFile(t, cert, filepath.Join(testDataDir, "server.crt")); err != nil {
		return err
	}
	env := []string{
		"POSTGRES_ADDR=" + serverAddr,
		"POSTGRES_PORT=" + serverPort,
		"ENCRYPTION_MODE=" + encryptionMode,
		"TESTDIR=" + testDataDir,
	}

	return protocolsUtils.RunDockerServer(t, "postgres", filepath.Join(testDataDir, "docker-compose.yml"), env, regexp.MustCompile(fmt.Sprintf(".*listening on IPv4 address \"0.0.0.0\", port %s", serverPort)), protocolsUtils.DefaultTimeout, 3)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// linkFile copies a file from src to dst, and sets up a cleanup function to remove the file when the test is done.
func linkFile(t testing.TB, src, dst string) error {
	t.Helper()
	_ = os.Remove(dst)
	if err := copyFile(src, dst); err != nil {
		return err
	}
	t.Cleanup(func() { os.Remove(dst) })
	return nil
}
