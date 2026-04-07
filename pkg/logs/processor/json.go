// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/google/uuid"
)

const nanoToMillis = 1000000

// JSONEncoder is a shared json encoder.
var JSONEncoder Encoder = &jsonEncoder{}

// JSONPayload is a shared JSON representation of a message
var JSONPayload = jsonPayload{}

// jsonEncoder transforms a message into a JSON byte array.
type jsonEncoder struct{}

// JSON representation of a message.
type jsonPayload struct {
	Message      ValidUtf8Bytes `json:"message"`
	Status       string         `json:"status"`
	Timestamp    int64          `json:"timestamp"`
	Hostname     string         `json:"hostname"`
	Service      string         `json:"service"`
	Source       string         `json:"ddsource"`
	Tags         string         `json:"ddtags"`
	DualSendUUID string         `json:"dual-send-uuid"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return errors.New("message passed to encoder isn't rendered")
	}

	// Save pre-encoded content and hostname for gRPC dual-send path.
	msg.PreEncodedContent = msg.GetContent()
	msg.MessageMetadata.Hostname = hostname

	// TODO: we should only send this if dual-send is enabled in future
	msg.DualSendUUID = uuid.NewString()

	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}
	msg.MessageMetadata.EncodedTimestampMs = ts.UnixNano() / nanoToMillis

	encoded, err := json.Marshal(jsonPayload{
		Message:      ValidUtf8Bytes(msg.GetContent()),
		Status:       msg.GetStatus(),
		Timestamp:    msg.MessageMetadata.EncodedTimestampMs,
		Hostname:     hostname,
		Service:      msg.Origin.Service(),
		Source:       msg.Origin.Source(),
		Tags:         msg.TagsToString(),
		DualSendUUID: msg.DualSendUUID,
	})

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
