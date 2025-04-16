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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	// KafkaOldPort is the port of the old kafka instance (v3.8), hard coded in docker-compose.yml for simplicity
	KafkaOldPort = "9082"
)

// RunServer runs a kafka server in a docker container
func RunServer(t testing.TB, serverAddr, serverPort string) error {
	env := []string{
		"KAFKA_ADDR=" + serverAddr,
		"KAFKA_PORT=" + serverPort,
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
	dockerCfg := dockerutils.NewComposeConfig("kafka",
		dockerutils.DefaultTimeout,
		dockerutils.DefaultRetries,
		scanner,
		env,
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	return dockerutils.Run(t, dockerCfg)
}
