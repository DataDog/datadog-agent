// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package replay is a component to run the dogstatsd capture/replay
package replay

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {

	// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
	IsOngoing() bool

	// StartCapture starts a TrafficCapture and returns an error in the event of an issue.
	StartCapture(p string, d time.Duration, compressed bool) (string, error)

	// StopCapture stops an ongoing TrafficCapture.
	StopCapture()

	// TODO: (components) pool manager should be injected as a component in the future.
	// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
	RegisterSharedPoolManager(p *packets.PoolManager[packets.Packet]) error

	// TODO: (components) pool manager should be injected as a component in the future.
	// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.f
	RegisterOOBPoolManager(p *packets.PoolManager[[]byte]) error

	// Enqueue enqueues a capture buffer so it's written to file.
	Enqueue(msg *CaptureBuffer) bool

	// GetStartUpError returns an error if TrafficCapture failed to start up
	GetStartUpError() error
}

// UnixDogstatsdMsg mirrors the exported fields of pkg/proto/pbgo/core/model.pb.go 'UnixDogstatsdMsg
// to avoid forcing the import of pbgo on every user of dogstatsd.
type UnixDogstatsdMsg struct {
	Timestamp     int64
	PayloadSize   int32
	Payload       []byte
	Pid           int32
	AncillarySize int32
	Ancillary     []byte
}

// CaptureBuffer holds pointers to captured packet's buffers (and oob buffer if required) and the protobuf
// message used for serialization.
type CaptureBuffer struct {
	Pb          UnixDogstatsdMsg
	Oob         *[]byte
	Pid         int32
	ContainerID string
	Buff        *packets.Packet
}

const (
	// GUID will be used as the GUID during capture replays
	// This is a magic number chosen for no particular reason other than the fact its
	// quite large an improbable to match an actual Group ID on any given box. We
	// need this number to identify replayed Unix socket ancillary credentials.
	GUID = 999888777
)
