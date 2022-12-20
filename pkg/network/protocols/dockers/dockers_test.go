// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)
// +build linux_bpf windows,npm

package dockers

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func tryConnectingKafka(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

func waitForKafka(ctx context.Context, zookeeperAddr string) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if tryConnectingKafka(zookeeperAddr) {
				return nil
			}
			time.Sleep(time.Second)
			continue
		}
	}
}

func TestDockersInCI(t *testing.T) {
	if runtime.GOOS != "linux" {
		return
	}

	envs := []string{
		fmt.Sprintf("KAFKA_ADDR=%s", "127.0.0.1"),
		"KAFKA_PORT=9092",
	}
	dir, _ := testutil.CurDir()
	cmd := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	cmd.Env = append(cmd.Env, envs...)
	go func() {
		if err := cmd.Run(); err != nil {
			fmt.Println("error", err)
		}
	}()

	t.Cleanup(func() {
		c := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "down", "--remove-orphans")
		c.Stdout = os.Stdout
		c.Stderr = os.Stdout
		c.Env = append(c.Env, envs...)
		_ = c.Run()
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	require.NoError(t, waitForKafka(ctx, fmt.Sprintf("%s:9092", "127.0.0.1")))
	cancel()
}
