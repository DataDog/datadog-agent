// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

// Strategy should contain all logic to send logs to a remote destination
// and forward them the next stage of the pipeline. In the logs pipeline,
// the strategy implementation should convert a stream of incoming Messages
// to a stream of Payloads that the sender can handle. A strategy is startable
// and stoppable so that the pipeline can manage it's lifecycle.
type Strategy interface {
	Start()
	Stop()
}
