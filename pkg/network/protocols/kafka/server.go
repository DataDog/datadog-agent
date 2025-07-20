// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package kafka

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	KafkaPort    = "9092"
	KafkaTLSPort = "9093"

	// KafkaOldPort is the port of the old kafka instance (v3.8), hard coded in docker-compose.yml for simplicity
	KafkaOldPort    = "9082"
	KafkaOldTLSPort = "9083"
)

func GetPort(kafkaVersion float32, tls bool) string {
	if kafkaVersion <= 3.8 {
		// Old kafka instance (v3.8) port, hard coded in docker-compose.yml for simplicity
		if tls {
			return KafkaOldTLSPort
		}
		return KafkaOldPort
	}
	if tls {
		// TLS port for the new kafka instance (v4.0)
		return KafkaTLSPort
	}
	// Non-TLS port for the new kafka instance (v4.0)
	return KafkaPort
}

// RunServer runs a kafka server in a docker container
// Ports are hard coded in docker-compose.yml, 9092 (9093 for tls) is the new kafka instance (v4.0) and 9082 (9083 for tls) is the old kafka instance (v3.8).
func RunServer(t testing.TB, serverAddr string) error {
	env := []string{
		"KAFKA_ADDR=" + serverAddr,
	}

	t.Helper()
	dir, _ := testutil.CurDir()

	// The JKS files need to be readable by others for the Kafka in the Docker
	// container to be able to use them. git doesn't save others' permissions
	// in the repo, so fix it up here.
	err := os.Chmod(filepath.Join(dir, "testdata/kafka.keystore.jks"), 0664)
	if err != nil {
		return err
	}

	err = os.Chmod(filepath.Join(dir, "testdata/kafka.truststore.jks"), 0664)
	if err != nil {
		return err
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile(`.*started \(kafka.server.KafkaRaftServer\).*`), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"kafka",
			scanner,
			dockerutils.WithEnv(env),
			dockerutils.WithTimeout(2*time.Minute),
		),
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}
