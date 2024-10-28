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

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
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

	return protocolsUtils.RunDockerServer(t, "kafka", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(`.*started \(kafka.server.KafkaServer\).*`), 1*time.Minute, 3)
}
