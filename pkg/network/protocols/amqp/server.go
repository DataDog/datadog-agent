// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package amqp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/stretchr/testify/require"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	User = "guest"
	Pass = "guest"
)

type logWriter struct {
	port         string
	callbackChan chan struct{}
	ignore       bool
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	if l.ignore {
		return len(p), nil
	}
	e := fmt.Sprintf("started TCP listener on [::]:%s", l.port)
	if strings.Contains(string(p), e) {
		l.callbackChan <- struct{}{}
		l.ignore = true
	}

	return len(p), nil
}

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
	writer := &logWriter{
		port:         serverPort,
		callbackChan: make(chan struct{}, 1),
		ignore:       false,
	}
	cmd.Stdout = writer
	cmd.Env = append(cmd.Env, env...)
	go func() {
		require.NoErrorf(t, cmd.Run(), "could not start amqp with docker-compose")
	}()

	cleanup := func() {
		c := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "down", "--remove-orphans")
		c.Env = append(c.Env, env...)
		_ = c.Run()
	}

	t.Cleanup(cleanup)

	for {
		select {
		case <-writer.callbackChan:
			fmt.Println("amqp server is ready")
			return
		case <-time.After(time.Second * 30):
			t.Fatal("failed to start amqp server")
		}
	}
}
