// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package client

// Destination sends a payload to a specific endpoint over a given network protocol.
type Destination interface {
	Send(payload []byte) error
	SendAsync(payload []byte)
}
