// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

func RunPostgresServer(t *testing.T, serverAddr string, serverPort string) {
	t.Helper()

	env := []string{
		"POSTGRES_ADDR=" + serverAddr,
		"POSTGRES_PORT=" + serverPort,
	}

	dir, _ := testutil.CurDir()

	protocolsUtils.RunDockerServer(t, "postgres", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(".*database system is ready to accept connections"))
}
