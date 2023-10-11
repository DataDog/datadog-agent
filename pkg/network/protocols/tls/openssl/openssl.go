// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openssl

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// RunServerOpenssl launches an openssl server.
func RunServerOpenssl(t *testing.T, serverPort string, clientCount int, args ...string) error {
	env := []string{
		"OPENSSL_PORT=" + serverPort,
		"CLIENTS=" + strconv.Itoa(clientCount),
	}
	if len(args) > 0 {
		env = append(env, "OPENSSL_ARGS="+args[0])
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "openssl-server", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile("ACCEPT"), protocolsUtils.DefaultTimeout)
}

// RunClientOpenssl launches an openssl client.
func RunClientOpenssl(t *testing.T, addr, port, args string) bool {
	command := []string{
		"docker", "run", "--network=host", "menci/archlinuxarm:base",
		"openssl", "s_client", "-connect", addr + ":" + port, args,
	}
	return protocolsUtils.RunHostServer(t, command, []string{}, regexp.MustCompile("Verify return code"))
}
