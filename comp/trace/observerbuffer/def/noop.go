// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerbuffer

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

type noopBuffer struct{}

// NewNoop returns a no-op observer buffer implementation.
func NewNoop() Component {
	return noopBuffer{}
}

func (n noopBuffer) AddTrace(_ *pb.TracerPayload)                  {}
func (n noopBuffer) AddProfile(_ ProfileData)                      {}
func (n noopBuffer) AddRawProfile(_ []byte, _ map[string][]string) {}

func (n noopBuffer) DrainTraces(_ uint32) ([]BufferedTrace, uint64, bool) {
	return nil, 0, false
}

func (n noopBuffer) DrainProfiles(_ uint32) ([]ProfileData, uint64, bool) {
	return nil, 0, false
}

func (n noopBuffer) Stats() BufferStats {
	return BufferStats{}
}
