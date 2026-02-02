// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package nodejs provides helpers to run nodejs HTTPs server.
package nodejs

import (
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
	"github.com/shirou/gopsutil/v4/process"
)

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

func linkFile(t *testing.T, src, dst string) error {
	t.Helper()
	_ = os.Remove(dst)
	if err := copyFile(src, dst); err != nil {
		return err
	}
	t.Cleanup(func() { os.Remove(dst) })
	return nil
}

// RunServerNodeJS launches an HTTPs server written in NodeJS.
func RunServerNodeJS(t *testing.T, key, cert, serverPort string) error {
	t.Helper()
	dir, _ := testutil.CurDir()
	if err := linkFile(t, key, dir+"/testdata/certs/srv.key"); err != nil {
		return err
	}
	if err := linkFile(t, cert, dir+"/testdata/certs/srv.crt"); err != nil {
		return err
	}
	env := []string{
		"ADDR=0.0.0.0",
		"PORT=" + serverPort,
		"CERTS_DIR=/v/certs",
		"TESTDIR=" + dir + "/testdata",
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile("Server running at https.*"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"nodejs-server",
			scanner,
			dockerutils.WithEnv(env),
		),
		path.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}

// GetNodeJSDockerPID returns the PID of the nodejs docker container.
func GetNodeJSDockerPID() (int64, error) {
	return dockerutils.GetMainPID("node-node-1")
}

// RunServerNodeJSUbuntu launches an HTTPs server written in NodeJS using the Ubuntu-based image.
// Ubuntu 22.04's nodejs package has SSL symbols bundled in libnode.so (not imported from libssl.so).
func RunServerNodeJSUbuntu(t *testing.T, key, cert, serverPort string) error {
	t.Helper()
	dir, _ := testutil.CurDir()

	// Create a temp directory for certs with proper ownership
	tmpDir, err := os.MkdirTemp("", "nodejs-certs-*")
	if err != nil {
		return err
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	if err := os.Chown(tmpDir, 1000, 1000); err != nil {
		return err
	}

	// Copy certs to temp dir and chown to 1000:1000
	keyDst := path.Join(tmpDir, "srv.key")
	certDst := path.Join(tmpDir, "srv.crt")
	if err := copyFile(key, keyDst); err != nil {
		return err
	}
	if err := os.Chown(keyDst, 1000, 1000); err != nil {
		return err
	}
	if err := copyFile(cert, certDst); err != nil {
		return err
	}
	if err := os.Chown(certDst, 1000, 1000); err != nil {
		return err
	}

	env := []string{
		"ADDR=0.0.0.0",
		"PORT=" + serverPort,
		"CERTS_DIR=/certs",
		"TESTDIR=" + dir + "/testdata",
		"CERTSDIR=" + tmpDir,
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile("Server running at https.*"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"nodejs-ubuntu-server",
			scanner,
			dockerutils.WithEnv(env),
		),
		path.Join(dir, "testdata", "docker-compose-ubuntu.yml"))
	return dockerutils.Run(t, dockerCfg)
}

// GetNodeJSUbuntuDockerPID returns the PID of the nodejs Ubuntu docker container.
func GetNodeJSUbuntuDockerPID() (int64, error) {
	mainPID, err := dockerutils.GetMainPID("node-ubuntu-node-1")
	if err != nil {
		return 0, err
	}
	proc, err := process.NewProcess(int32(mainPID))
	if err != nil {
		return 0, err
	}
	children, err := proc.Children()
	if err != nil {
		return 0, err
	}
	if len(children) == 0 {
		return 0, errors.New("no children found for tini process")
	}
	return int64(children[0].Pid), nil
}
