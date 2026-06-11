// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

// PayloadSink is the boundary between the stateful encoder and the transport:
// the encoder hands it destination-neutral *Payloads and the sink delivers them.
// Sender (one destination) and Fanout (many) both implement it.
//
// Submit may block to apply back-pressure but never drops a payload (a dropped
// payload loses its Defines, breaking later references on the stream); it
// returns a non-nil error only on shutdown.
type PayloadSink interface {
	Submit(*Payload) error
}
