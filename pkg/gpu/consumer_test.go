// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
)

func TestConsumerCanStartAndStop(t *testing.T) {
	handler := ddebpf.NewRingBufferHandler(consumerChannelSize)
	cfg := config.NewConfig()
	consumer := newCudaEventConsumer(handler, cfg)

	consumer.Start()
	require.Eventually(t, func() bool { return consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)

	consumer.Stop()
	require.Eventually(t, func() bool { return !consumer.running.Load() }, 100*time.Millisecond, 10*time.Millisecond)
}
