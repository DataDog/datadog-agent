// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Aggregator is the interface for multiline log processing.
// Both combining and detecting implementations satisfy this interface.
type Aggregator interface {
	Process(msg *message.Message, label Label)
	Flush()
	IsEmpty() bool
}
