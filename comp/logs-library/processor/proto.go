// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/agent-payload/v5/pb"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// ProtoEncoder is a shared proto encoder.
var ProtoEncoder Encoder = &protoEncoder{}

// protoEncoder transforms a message into a protobuf byte array.
type protoEncoder struct {
	useContainerTimestamp bool
}

// NewProtoEncoder returns a proto encoder configured to optionally use container-provided timestamps.
func NewProtoEncoder(useContainerTimestamp bool) Encoder {
	return &protoEncoder{useContainerTimestamp: useContainerTimestamp}
}

// Encode encodes a message into a protobuf byte array.
func (p *protoEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return errors.New("message passed to encoder isn't rendered")
	}

	ts := msg.ServerlessExtra.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
		if msg.ParsingExtra.Timestamp != "" && p.useContainerTimestamp {
			if logTime, err := time.Parse(time.RFC3339Nano, msg.ParsingExtra.Timestamp); err == nil {
				ts = logTime
			}
		}
	}

	log := &pb.Log{
		Message:   toValidUtf8(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: ts.UnixNano(),
		Hostname:  hostname,
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      msg.Tags(),
	}
	encoded, err := log.Marshal()

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
