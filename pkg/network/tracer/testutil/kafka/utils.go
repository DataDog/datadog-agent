// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func tryConnectingKafka(addr string) bool {
	dialer := &kafka.Dialer{
		Timeout: 10 * time.Second,
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	_, err = conn.ApiVersions()
	return err == nil
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

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}

func CurDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

func PullKafkaDockers() error {
	dir, _ := CurDir()
	envs := []string{
		"KAFKA_ADDR=127.0.0.1",
		"KAFKA_PORT=9092",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker-compose", "-f", dir+"/testdata/docker-compose.yml", "pull")
	cmd.Env = append(cmd.Env, envs...)
	return cmd.Run()
}

func RunKafkaServers(t *testing.T, serverAddr string) {
	t.Helper()
	envs := []string{
		fmt.Sprintf("KAFKA_ADDR=%s", serverAddr),
		"KAFKA_PORT=9092",
	}
	dir, _ := CurDir()
	cmd := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "up")
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
	require.NoError(t, waitForKafka(ctx, fmt.Sprintf("%s:9092", serverAddr)))
	cancel()
}
