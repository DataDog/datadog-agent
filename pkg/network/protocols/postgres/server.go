// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package postgres provides a simple wrapper around 3rd party postgres client.
package postgres

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunServer runs a postgres server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string) error {
	t.Helper()

	env := []string{
		"POSTGRES_ADDR=" + serverAddr,
		"POSTGRES_PORT=" + serverPort,
	}

	dir, _ := testutil.CurDir()

	return protocolsUtils.RunDockerServer(t, "postgres", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(`.*\[1].*database system is ready to accept connections`), protocolsUtils.DefaultTimeout, 3)
}
