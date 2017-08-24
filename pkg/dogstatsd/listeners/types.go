// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package listeners

// Packet reprensents a statsd packet ready to process,
// with its origin metadata if applicable.
type Packet struct {
	Contents []byte // Contents, might contain several messages
	Origin   string // Origin container if identified
}

// StatsdListener opens a communication channel to get statsd packets in.
type StatsdListener interface {
	Listen()
	Stop()
}
