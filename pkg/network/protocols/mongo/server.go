// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mongo

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

const (
	User = "root"
	Pass = "password"
)

func RunServer(t testing.TB, serverAddress, serverPort string) error {
	env := []string{
		"MONGO_ADDR=" + serverAddress,
		"MONGO_PORT=" + serverPort,
		"MONGO_USER=" + User,
		"MONGO_PASSWORD=" + Pass,
	}
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "mongo", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(fmt.Sprintf(".*Waiting for connections.*port.*:%s.*", serverPort)), protocolsUtils.DefaultTimeout)
}
