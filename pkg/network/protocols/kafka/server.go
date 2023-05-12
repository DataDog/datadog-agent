// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kafka

import (
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolsUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
)

func RunServer(t testing.TB, serverAddr, serverPort string) error {
	env := []string{
		"KAFKA_ADDR=" + serverAddr,
		"KAFKA_PORT=" + serverPort,
	}

	t.Helper()
	dir, _ := testutil.CurDir()
	return protocolsUtils.RunDockerServer(t, "kafka", dir+"/testdata/docker-compose.yml", env, regexp.MustCompile(".*started \\(kafka.server.KafkaServer\\).*"), protocolsUtils.DefaultTimeout)
}
