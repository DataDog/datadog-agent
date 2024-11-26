// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mongo

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	// User is the username to use for authentication
	User = "root"
	// Pass is the password to use for authentication
	Pass = "password"
)

// RunServer runs a mongo server in a docker container
func RunServer(t testing.TB, serverAddress, serverPort string) error {
	env := []string{
		"MONGO_ADDR=" + serverAddress,
		"MONGO_PORT=" + serverPort,
		"MONGO_USER=" + User,
		"MONGO_PASSWORD=" + Pass,
	}
	dir, _ := testutil.CurDir()
	dockerCfg := dockerutils.NewComposeConfig("mongo",
		dockerutils.DefaultTimeout,
		dockerutils.DefaultRetries,
		regexp.MustCompile(fmt.Sprintf(".*Waiting for connections.*port.*:%s.*", serverPort)),
		env,
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}
