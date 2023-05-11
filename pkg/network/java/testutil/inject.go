// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"bytes"
	"os/exec"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	"github.com/stretchr/testify/require"
)

// RunJavaVersion run class under java version
func RunJavaVersion(t testing.TB, version string, class string, waitForParam ...*regexp.Regexp) error {
	t.Helper()
	var waitFor *regexp.Regexp
	if len(waitForParam) == 0 {
		// test if injection happen
		waitFor = regexp.MustCompile(`loading TestAgentLoaded\.agentmain.*`)
	} else {
		waitFor = waitForParam[0]
	}

	dir, _ := testutil.CurDir()
	docker0IpAddr, err := exec.Command("docker", "network", "inspect", "bridge", "--format='{{(index .IPAM.Config 0).Gateway}}'").CombinedOutput()
	require.NoErrorf(t, err, "failed to get docker0 ip address")
	addr := string(bytes.Trim(docker0IpAddr, "'\n"))
	env := []string{
		"IMAGE_VERSION=" + version,
		"ENTRYCLASS=" + class,
		"EXTRA_HOSTS=host.docker.internal:" + addr,
	}
	return protocolsUtils.RunDockerServer(t, version, dir+"/../testdata/docker-compose.yml", env, waitFor, protocolsUtils.DefaultTimeout)
}
