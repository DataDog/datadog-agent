// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package def contains the definitions for the batch sender component
package def

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/config"
	"github.com/DataDog/datadog-agent/comp/logs-library/message"
)

// BatchSender is the interface for the batch sender pipeline - a logs pipeline that does not include any processing
type BatchSender interface {
	Start()
	Stop()
	GetInputChan() chan *message.Message
}

// FactoryComponent is the interface for the batch sender factory
type FactoryComponent interface {
	NewBatchSender(
		endpoints *config.Endpoints,
		destinationsContext *client.DestinationsContext,
		eventType string,
		contentType string,
		category string,
		disableBatching bool,
		pipelineID int,
	) BatchSender
}
