// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

// GRPCEncoder is a shared gRPC encoder that encodes messages as Datum->Log->raw.
var GRPCEncoder Encoder = &grpcEncoder{}

// grpcEncoder transforms a message into a protobuf Datum byte array for gRPC transmission.
type grpcEncoder struct{}

// Encode encodes a message into a protobuf Datum byte array.
// The structure is: Datum -> Log -> raw content
func (g *grpcEncoder) Encode(msg *message.Message, _ string) error {
	if msg.State != message.StateRendered {
		return fmt.Errorf("message passed to encoder isn't rendered")
	}

	// Get timestamp - prefer message timestamp if available
	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}

	// Create the Log message using stateful_encoding.proto definitions
	log := &statefulpb.Log{
		Timestamp: uint64(ts.UnixMilli()),
		Content: &statefulpb.Log_Raw{
			Raw: toValidUtf8(msg.GetContent()),
		},
		// TODO: add hostname/other tags
	}

	// Wrap the Log in a Datum
	datum := &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: log,
		},
	}

	// Instead of marshaling, just store the Datum in the message
	msg.SetGRPCDatum(datum)
	return nil
}
