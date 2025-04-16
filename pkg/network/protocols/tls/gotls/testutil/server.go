// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// RunServer runs a go-httpbin server in a docker container.
func RunServer(t testing.TB, serverPort string) error {
	env := []string{
		"HTTPS_PORT=" + serverPort,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	scanner, err := globalutils.NewScanner(regexp.MustCompile("go-httpbin listening on https://0.0.0.0:8080"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")
	dockerCfg := dockerutils.NewComposeConfig("https-gotls",
		dockerutils.DefaultTimeout,
		dockerutils.DefaultRetries,
		scanner,
		env,
		dir+"/../testdata/docker-compose.yml")
	return dockerutils.Run(t, dockerCfg)

}
