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

const (
	User = "guest"
	Pass = "guest"
)

func RunAmqpServer(t *testing.T, serverAddr, serverPort string) {
	env := []string{
		"AMQP_ADDR=" + serverAddr,
		"AMQP_PORT=" + serverPort,
		"USER=" + User,
		"PASS=" + Pass,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	protocolsUtils.RunDockerServer(t, "amqp", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(fmt.Sprintf(".*started TCP listener on .*%s.*", serverPort)))
}
