// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// JSONServerlessInitEncoder is a custom encoder used by serverless-init
// (Google Cloud Run, Azure Container Apps, etc.) for improved performance.
var JSONServerlessInitEncoder Encoder = &jsonServerlessInitEncoder{}

// jsonServerlessInitEncoder transforms a message into a JSON byte array.
// It caches the tags string since tags are constant in serverless-init environments.
type jsonServerlessInitEncoder struct {
	cachedTags string
}

// JSON representation of a message for serverless-init.
type jsonServerlessInitPayload struct {
	Message   string `json:"message"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
	Hostname  string `json:"hostname"`
	Service   string `json:"service,omitempty"`
	Source    string `json:"ddsource"`
	Tags      string `json:"ddtags"`
}

// Encode encodes a message into a JSON byte array.
func (j *jsonServerlessInitEncoder) Encode(msg *message.Message, hostname string) error {
	if msg.State != message.StateRendered {
		return fmt.Errorf("message passed to encoder isn't rendered")
	}

	ts := time.Now().UTC()
	if !msg.ServerlessExtra.Timestamp.IsZero() {
		ts = msg.ServerlessExtra.Timestamp
	}

	// Cache tags on first use since they're constant in serverless-init
	if j.cachedTags == "" {
		j.cachedTags = msg.TagsToString()
	}

	encoded, err := json.Marshal(jsonServerlessInitPayload{
		Message:   toValidUtf8(msg.GetContent()),
		Status:    msg.GetStatus(),
		Timestamp: ts.UnixNano() / nanoToMillis,
		Hostname:  hostname,
		Service:   msg.Origin.Service(),
		Source:    msg.Origin.Source(),
		Tags:      j.cachedTags,
	})

	if err != nil {
		return fmt.Errorf("can't encode the message: %v", err)
	}

	msg.SetEncoded(encoded)
	return nil
}
