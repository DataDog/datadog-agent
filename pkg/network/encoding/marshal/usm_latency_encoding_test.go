// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package marshal

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
)

func TestCompareLatencyEncoding(t *testing.T) {
	latencies, err := ddsketch.NewDefaultDDSketch(0.01)
	require.NoError(t, err)
	for i := -110; i < 1000; i++ {
		require.NoError(t, latencies.Add(float64(i)))
	}

	protobufBlob, err := proto.Marshal(latencies.ToProto())
	require.NoError(t, err)
	protobufLatencies := &sketchpb.DDSketch{}
	require.NoError(t, proto.Unmarshal(protobufBlob, protobufLatencies))

	gostreamerBlob := bytes.NewBuffer(nil)
	latencies.EncodeProto(gostreamerBlob)
	gostreamerLatencies := &sketchpb.DDSketch{}

	require.NoError(t, proto.Unmarshal(gostreamerBlob.Bytes(), gostreamerLatencies))
}
