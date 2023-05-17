// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mysql

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

const (
	User = "root"
	Pass = "root"
)

func RunServer(t testing.TB, serverAddr, serverPort string) error {
	env := []string{
		"MYSQL_ADDR=" + serverAddr,
		"MYSQL_PORT=" + serverPort,
		"MYSQL_ROOT_PASS=" + Pass,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "MYSQL", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(fmt.Sprintf(".*ready for connections.*port: %s.*", serverPort)), 10*time.Minute)
}
