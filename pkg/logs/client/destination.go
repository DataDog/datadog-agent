// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// Destination sends a payload to a specific endpoint over a given network protocol.
type Destination interface {
	Start(input chan *message.Payload, output chan *message.Payload) (stopChan chan struct{})
	GetIsRetrying() bool
}
