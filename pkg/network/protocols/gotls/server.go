// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gotls

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

func RunServer(t testing.TB, serverPort string) error {
	env := []string{
		"HTTPS_PORT=" + serverPort,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "https-gotls", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile("go-httpbin listening on https://0.0.0.0:8080"), protocolsUtils.DefaultTimeout)
}
