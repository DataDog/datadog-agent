// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package tracer

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// RunDNSWorkload runs a CoreDNS server and Python client in docker containers
// The Python client continuously queries my-server.local against the CoreDNS server in a single process
func RunDNSWorkload(t testing.TB) error {
	t.Helper()

	curDir, err := usmtestutil.CurDir()
	require.NoError(t, err)

	env := []string{
		"TESTDIR=" + curDir,
	}

	// this regex indicates that the Python client got a response from CoreDNS
	scanner, err := globalutils.NewScanner(regexp.MustCompile(`.*Address: 1\.2\.3\.4.*`), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"dnsworkload",
			scanner,
			dockerutils.WithEnv(env),
		),
		filepath.Join(curDir, "testdata", "dnsworkload", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}
