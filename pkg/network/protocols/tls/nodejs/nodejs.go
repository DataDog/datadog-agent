// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodejs provides helpers to run nodejs HTTPs server.
package nodejs

import (
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
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
	return protocolsUtils.RunDockerServer(t, "nodejs-server", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile("Server running at https.*"), protocolsUtils.DefaultTimeout, 3)
}

// GetNodeJSDockerPID returns the PID of the nodejs docker container.
func GetNodeJSDockerPID() (int64, error) {
	return protocolsUtils.GetDockerPID("node-node-1")
}
