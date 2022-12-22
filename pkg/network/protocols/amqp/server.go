// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"fmt"
	"github.com/streadway/amqp"
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/stretchr/testify/require"
)

func Connect(serverAddr, serverPort string) *amqp.Connection {
	conn, err := amqp.Dial(fmt.Sprintf("amqp://guest:guest@%s:%s/", serverAddr, serverPort))
	failOnError(err, "Failed to connect to RabbitMQ")
	//defer conn.Close()
	return conn
}

func RunAmqpServer(t *testing.T, serverAddr, serverPort string) {
	t.Helper()

	env := []string{
		"AMQP_ADDR=" + serverAddr,
		"AMQP_PORT=" + serverPort,
	}
	dir, _ := testutil.CurDir()
	cmd := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "up", "-d")
	cmd.Env = append(cmd.Env, env...)
	require.NoErrorf(t, cmd.Run(), "could not start amqp with docker-compose")

	t.Cleanup(func() {
		c := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "down", "--remove-orphans")
		c.Env = append(c.Env, env...)
		_ = c.Run()
	})
}
