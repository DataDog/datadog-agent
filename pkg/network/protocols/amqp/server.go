// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

// This const block should have a comment or be unexported
const (
	User = "guest"
	Pass = "guest"
)

// RunServer exported function should have comment or be unexported
func RunServer(t testing.TB, serverAddr, serverPort string) error {
	env := []string{
		"AMQP_ADDR=" + serverAddr,
		"AMQP_PORT=" + serverPort,
		"USER=" + User,
		"PASS=" + Pass,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "amqp", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(fmt.Sprintf(".*started TCP listener on .*%s.*", serverPort)), protocolsUtils.DefaultTimeout)
}
