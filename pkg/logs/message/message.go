// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content  []byte
	Origin   *Origin
	Severity []byte
}

// NewMessage returns a new message
func NewMessage(content []byte, origin *Origin, severity []byte) *Message {
	return &Message{
		Content:  content,
		Origin:   origin,
		Severity: severity,
	}
}
