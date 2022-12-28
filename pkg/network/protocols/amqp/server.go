// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

const (
	User = "guest"
	Pass = "guest"
)

func RunAmqpServer(t *testing.T, serverAddr, serverPort string) {
	t.Helper()

	env := []string{
		"AMQP_ADDR=" + serverAddr,
		"AMQP_PORT=" + serverPort,
		"USER=" + User,
		"PASS=" + Pass,
	}
	dir, _ := testutil.CurDir()
	cmd := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "up")
	patternScanner := protocolsUtils.NewScanner(regexp.MustCompile(fmt.Sprintf(".*started TCP listener on .*%s.*", serverPort)), make(chan struct{}, 1))

	cmd.Stdout = patternScanner
	cmd.Env = append(cmd.Env, env...)
	go func() {
		require.NoErrorf(t, cmd.Run(), "could not start amqp with docker-compose")
	}()

	t.Cleanup(func() {
		c := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "down", "--remove-orphans")
		c.Env = append(c.Env, env...)
		_ = c.Run()
	})

	for {
		select {
		case <-patternScanner.DoneChan:
			fmt.Println("amqp server is ready")
			return
		case <-time.After(time.Second * 30):
			t.Fatal("failed to start amqp server")
		}
	}
}
