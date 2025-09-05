// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fake

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

type MockGRPCServer struct {
	statefulpb.UnimplementedStatefulLogsServiceServer

	// Control behavior
	shouldFailSend   bool
	shouldFailRecv   bool
	shouldDisconnect bool
	responseDelay    time.Duration
	batchResponses   map[int32]statefulpb.BatchStatus_Status
	mu               sync.RWMutex

	// Track what was received
	receivedBatches []*statefulpb.StatefulBatch
	activeStreams   []statefulpb.StatefulLogsService_LogsStreamServer
	streamsMu       sync.RWMutex
}
