// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	log.SetupLogger(seelog.Default, "trace")
	os.Exit(m.Run())
}

func TestGetDDAgentClientTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := GetDDAgentClient(ctx, "127.0.0.1", "5001")
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestGetDDAgentClientWithCmdPort0(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := GetDDAgentClient(ctx, "127.0.0.1", "-1")
	assert.NotNil(t, err)
	assert.Equal(t, "grpc client disabled via cmd_port: -1", err.Error())
}
