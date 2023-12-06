// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package redis provides a Redis client to interact with a Redis server.
package redis

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunServer runs a Redis server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string) error {
	env := []string{
		"REDIS_ADDR=" + serverAddr,
		"REDIS_PORT=" + serverPort,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "redis", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(".*Ready to accept connections"), protocolsUtils.DefaultTimeout, 3)
}
